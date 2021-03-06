package events

import (
	"regexp"

	gerrit "github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/gerrit-slack/gerritssh"
	"github.com/levenlabs/gerrit-slack/project"
	llog "github.com/levenlabs/go-llog"
)

// EventHandler takes a given Event and generates a Message
type EventHandler interface {
	// Type should return the type the handler handles
	Type() string

	// Ignore should return true if the event should be ignored
	Ignore(gerritssh.Event, project.Config) (bool, error)

	// Message should return a Message for the event
	Message(gerritssh.Event, project.Config, *gerrit.Client, MessageEnricher) (Message, error)
}

// MessageEnricher is used when building a message to mention a user
type MessageEnricher interface {
	// MentionUser takes an email and name and returns either a mention or their
	// name
	MentionUser(string, string) string
}

var handlers = map[string]EventHandler{}

func register(typ string, h EventHandler) {
	handlers[typ] = globalWrapper{h}
}

// Handler returns a registered handler for the sent event
func Handler(e gerritssh.Event, _ project.Config) (EventHandler, bool) {
	h, ok := handlers[e.Type]
	return h, ok
}

func regexMatch(reg, val string) (bool, error) {
	if reg == "" {
		return false, nil
	}
	r, err := regexp.Compile(reg)
	if err != nil {
		return false, llog.ErrWithKV(err, llog.KV{"regex": reg})
	}
	return r.MatchString(val), nil
}

type globalWrapper struct {
	EventHandler
}

// Ignore implements the EventHandler interface
func (w globalWrapper) Ignore(e gerritssh.Event, pcfg project.Config) (bool, error) {
	// if we're not enabled, ignore
	if !pcfg.Enabled {
		return true, nil
	}
	// if the change is still private, ignore
	if pcfg.IgnorePrivatePatchSet && e.Change.Private {
		return true, nil
	}
	// if the change is still wip, ignore
	if pcfg.IgnoreWipPatchSet && e.Change.WIP {
		return true, nil
	}
	return w.EventHandler.Ignore(e, pcfg)
}

// Message implements the EventHandler interface
func (w globalWrapper) Message(e gerritssh.Event, pcfg project.Config, c *gerrit.Client, me MessageEnricher) (Message, error) {
	m, err := w.EventHandler.Message(e, pcfg, c, me)
	if err == nil {
		if m.Channel == "" {
			m.Channel = pcfg.Channel
		}
		if m.Color == "" {
			m.Color = "good"
			if e.Change.Status == gerritssh.ChangeStatusMerged || e.Change.Status == gerritssh.ChangeStatusAbandoned {
				m.Color = "danger"
			}
		}
	}
	return m, err
}
