package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/go-ini/ini"
	"github.com/nlopes/slack"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/levenlabs/gerrit-slack/events"
	"github.com/levenlabs/gerrit-slack/gerritssh"
	"github.com/levenlabs/gerrit-slack/project"

	"github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/go-llog"
)

var sshRetryDelay = 3 * time.Second

type config struct {
	HTTPAddress    string `ini:"http-address"`
	SSHAddress     string `ini:"ssh-address"`
	Username       string `ini:"username"`
	Password       string `ini:"password"`
	PrivateKeyPath string `ini:"private-key-path"`
	HostKey        string `ini:"host-key"`
	DebugEvents    string `ini:"debug-events"`
	SlackToken     string `ini:"slack-token"`
}

func main() {
	cp := flag.String("config", "./slack.config", "path to ini-formatted config file")
	ll := flag.String("log-level", "info", "the log level to set on llog")
	flag.Parse()

	err := llog.SetLevelFromString(*ll)
	if err != nil {
		llog.Fatal("invalid log-level", llog.ErrKV(err))
	}

	var cfg config
	f, err := ini.Load(*cp)
	if err != nil {
		llog.Fatal("error reading config file", llog.ErrKV(err), llog.KV{"path": *cp})
	}
	if err := f.Section("gerrit").MapTo(&cfg); err != nil {
		llog.Fatal("error parsing config", llog.ErrKV(err), llog.KV{"path": *cp})
	}

	client, err := gerrit.NewClient(cfg.HTTPAddress, nil)
	if err != nil {
		llog.Fatal("error creating gerrit client", llog.ErrKV(err))
	}
	client.Authentication.SetBasicAuth(cfg.Username, cfg.Password)

	// make sure that the client works
	if _, _, err := client.Accounts.GetAccount("self"); err != nil {
		llog.Fatal("error validating gerrit client", llog.ErrKV(err))
	}
	llog.Info("connected to rest api")

	pk, err := ioutil.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		llog.Fatal("unable to read private key", llog.ErrKV(err))
	}
	sshc, err := gerritssh.NewClient(cfg.SSHAddress, cfg.Username, pk, []byte(cfg.HostKey))
	if err != nil {
		llog.Fatal("error creating ssh client", llog.ErrKV(err))
	}

	if cfg.DebugEvents != "" {
		llog.Info("debugging events")
		go debugEvents(cfg.DebugEvents, sshc)
	}
	// add a buffer so we don't overflow the ssh buffer trying to handle/submit
	sch := make(chan webhookSubmit, 10)
	go webhookSubmitter(sch)
	ech := make(chan gerritssh.Event, 10)
	go listenForEvents(client, ech, sch, cfg.SlackToken)

	llog.Info("streaming events")
	for {
		if err := sshc.StreamEvents(context.Background(), ech); err != nil {
			llog.Error("error streaming events", llog.ErrKV(err))
		}
		time.Sleep(sshRetryDelay)
	}
}

// SlackState holds various slack metadata that can be used to improve messages
type slackState struct {
	emailToID map[string]string
	refreshed time.Time
	sapi      *slack.Client
}

func (s *slackState) refresh() error {
	if s.sapi == nil {
		return nil
	}
	us, err := s.sapi.GetUsers()
	if err != nil {
		return err
	}
	emailToID := map[string]string{}
	for _, u := range us {
		if u.Profile.Email != "" {
			emailToID[strings.ToLower(u.Profile.Email)] = u.ID
		}
	}
	llog.Debug("loaded users from slack", llog.KV{"numUsers": len(emailToID)})
	s.emailToID = emailToID
	s.refreshed = time.Now()
	return nil
}

func (s *slackState) refreshIfNecessary() error {
	if s.sapi == nil {
		return nil
	}
	if time.Since(s.refreshed) > time.Hour {
		return s.refresh()
	}
	return nil
}

// MentionUser either returns just their name or it @ mentions them
// MentionUser implements the events.MessageEnricher interface
func (s *slackState) MentionUser(email string, name string) string {
	llog.Debug("lloking up user", llog.KV{"email": email})
	id, ok := s.emailToID[strings.ToLower(email)]
	if ok {
		return fmt.Sprintf("<@%s>", id)
	}
	return name
}

func listenForEvents(client *gerrit.Client, ech <-chan gerritssh.Event, sch chan webhookSubmit, token string) {
	var state slackState
	if token != "" {
		state.sapi = slack.New(token)
	}
	if err := state.refresh(); err != nil {
		llog.Fatal("failed to load slack metadata", llog.ErrKV(err))
	}

	for e := range ech {
		go func(e gerritssh.Event) {
			var pcfg project.Config
			if e.Change.Project != "" {
				var err error
				pcfg, err = project.LoadConfig(client, e.Change.Project)
				if err != nil {
					llog.Error("error loading config", llog.ErrKV(err), e.KV())
					return
				}
			}
			h, ok := events.Handler(e, pcfg)
			if !ok {
				llog.Info("no handlers for event", e.KV())
				return
			}
			ignore, err := h.Ignore(e, pcfg)
			if err != nil {
				llog.Error("error handling event", llog.ErrKV(err), e.KV(), llog.KV{"handler": h.Type()})
				return
			}
			if ignore {
				return
			}
			if err := state.refreshIfNecessary(); err != nil {
				llog.Error("error refreshing slack metadata", llog.ErrKV(err))
			}
			msg, err := h.Message(e, pcfg, client, &state)
			if err != nil {
				llog.Error("error generating message for event", llog.ErrKV(err), e.KV(), llog.KV{"handler": h.Type()})
				return
			}
			sch <- webhookSubmit{
				Message:    msg,
				WebhookURL: pcfg.WebhookURL,
				SourceType: e.Type,
			}
		}(e)
	}
}

type webhookSubmit struct {
	events.Message
	WebhookURL string
	SourceType string
}

func webhookSubmitter(sch <-chan webhookSubmit) {
	var pendingMessages []webhookSubmit

	publish := func(s webhookSubmit) bool {
		if s.WebhookURL == "" {
			return true
		}
		b, err := json.Marshal(s.Message)
		if err != nil {
			llog.Error("error marshalling message", llog.ErrKV(err))
			// pretend it worked because we can't magically marshal it later
			return true
		}
		resp, err := http.Post(s.WebhookURL, "application/json", bytes.NewBuffer(b))
		if err != nil {
			llog.Error("error posting to slack webhook", llog.ErrKV(err), llog.KV{"url": s.WebhookURL})
			return false
		}
		defer resp.Body.Close()
		kv := llog.KV{
			"channel": s.Channel,
			"url":     s.WebhookURL,
			"source":  s.SourceType,
		}
		switch resp.StatusCode {
		case http.StatusOK:
			llog.Info("posted to slack channel", kv)
		case http.StatusNotFound:
			llog.Error("slack channel does not exist", kv)
		case http.StatusGone:
			llog.Error("slack channel is archived", kv)
		default:
			var sbody string
			body, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				sbody = string(body)
				if len(sbody) > 250 {
					sbody = sbody[:250]
				}
			}
			llog.Error("unknown error posting to slack", kv, llog.KV{
				"status": resp.StatusCode,
				"body":   sbody,
			})
			return false
		}
		return true
	}
	// retry pending messages every minute
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if len(pendingMessages) > 0 {
				var newPend []webhookSubmit
				for _, s := range pendingMessages {
					if !publish(s) {
						newPend = append(newPend, s)
					}
				}
				pendingMessages = newPend
			}
		case s := <-sch:
			if !publish(s) {
				pendingMessages = append(pendingMessages, s)
			}
		}
	}
}

// todo: this is very similar to gerritssh.Client.StreamEvents
func debugEvents(p string, sshc *gerritssh.Client) {
	log := &lumberjack.Logger{
		Filename:   p,
		MaxSize:    100, // in MB
		MaxBackups: 3,   // keep at most 3 files
	}
	innerDebug := func() error {
		sess, err := sshc.Dial()
		if err != nil {
			llog.Error("error connecting to gerrit over ssh", llog.ErrKV(err))
			return err
		}
		sout, err := sess.StdoutPipe()
		if err != nil {
			llog.Error("error getting debug ssh stdout", llog.ErrKV(err))
			return err
		}
		sos := bufio.NewScanner(sout)
		runCh := make(chan error, 1)
		go func() {
			runCh <- sess.Run("gerrit stream-events")
		}()
		readCh := make(chan error, 1)
		go func() {
			for sos.Scan() {
				_, err := fmt.Fprintf(log, "%s: %s\n", time.Now().Format(time.RFC822), string(sos.Bytes()))
				if err != nil {
					llog.Error("error writing to debug buffer", llog.ErrKV(err))
				}
			}
			readCh <- sos.Err()
		}()
		select {
		case err = <-runCh:
			close(runCh)
		case err = <-readCh:
			close(readCh)
		}
		sess.Close()
		<-runCh
		<-readCh
		// ensure there's some error that's returned
		if err == nil {
			err = &ssh.ExitMissingError{}
		}
		return err
	}
	for {
		if err := innerDebug(); err != nil {
			llog.Error("error streaming debug events", llog.ErrKV(err))
		}
		time.Sleep(sshRetryDelay)
	}
}
