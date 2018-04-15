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
	"time"

	"github.com/go-ini/ini"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/levenlabs/gerrit-slack/events"
	"github.com/levenlabs/gerrit-slack/gerritssh"
	"github.com/levenlabs/gerrit-slack/project"

	"github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/go-llog"
)

type config struct {
	HTTPAddress    string `ini:"http-address"`
	SSHAddress     string `ini:"ssh-address"`
	Username       string `ini:"username"`
	Password       string `ini:"password"`
	PrivateKeyPath string `ini:"private-key-path"`
	HostKey        string `ini:"host-key"`
	DebugEvents    string `ini:"debug-events"`
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
	go listenForEvents(client, ech, sch)

	llog.Info("streaming events")
	if err := sshc.StreamEvents(context.Background(), ech); err != nil {
		llog.Fatal("error streaming events", llog.ErrKV(err))
	}
}

func listenForEvents(client *gerrit.Client, ech <-chan gerritssh.Event, sch chan webhookSubmit) {
	for e := range ech {
		var pcfg project.Config
		if e.Change.Project != "" {
			var err error
			pcfg, err = project.LoadConfig(client, e.Change.Project)
			if err != nil {
				llog.Error("error loading config", llog.ErrKV(err), e.KV())
				continue
			}
		}
		h, ok := events.Handler(e, pcfg)
		if !ok {
			llog.Info("no handlers for event", e.KV())
			continue
		}
		ignore, err := h.Ignore(e, pcfg)
		if err != nil {
			llog.Error("error handling event", llog.ErrKV(err), e.KV(), llog.KV{"handler": h.Type()})
			continue
		}
		if ignore {
			continue
		}
		msg, err := h.Message(e, pcfg, client)
		if err != nil {
			llog.Error("error generating message for event", llog.ErrKV(err), e.KV(), llog.KV{"handler": h.Type()})
			continue
		}
		sch <- webhookSubmit{
			Message:    msg,
			WebhookURL: pcfg.WebhookURL,
		}
	}
}

type webhookSubmit struct {
	events.Message
	WebhookURL string
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

func debugEvents(p string, sshc *gerritssh.Client) {
	sess, err := sshc.Dial()
	if err != nil {
		llog.Fatal("error connecting to gerrit over ssh", llog.ErrKV(err))
	}
	sout, err := sess.StdoutPipe()
	if err != nil {
		llog.Fatal("error gerrit ssh stdout", llog.ErrKV(err))
	}
	log := &lumberjack.Logger{
		Filename:   p,
		MaxSize:    100, // in MB
		MaxBackups: 3,   // keep at most 3 files
	}
	sos := bufio.NewScanner(sout)
	if err := sess.Start("gerrit stream-events"); err != nil {
		llog.Fatal("error streaming events", llog.ErrKV(err))
	}
	for sos.Scan() {
		_, err := fmt.Fprintf(log, "%s: %s\n", time.Now().Format(time.RFC822), string(sos.Bytes()))
		if err != nil {
			llog.Fatal("error writing to debug buffer", llog.ErrKV(err))
		}
	}
	if err := sos.Err(); err != nil {
		llog.Fatal("error scanning for ssh output", llog.ErrKV(err))
	}
}
