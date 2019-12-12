package events

import (
	"fmt"

	"github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/gerrit-slack/gerritssh"
	"github.com/levenlabs/gerrit-slack/project"
)

func init() {
	var h ChangeMerged
	register(h.Type(), h)
}

// ChangeMerged handles the change-merged event
type ChangeMerged struct{}

// Type implements the EventHandler interface
func (ChangeMerged) Type() string {
	return gerritssh.EventTypeChangeMerged
}

// Ignore implements the EventHandler interface
func (ChangeMerged) Ignore(e gerritssh.Event, pcfg project.Config) (bool, error) {
	return !pcfg.PublishOnChangeMerged, nil
}

// Message implements the EventHandler interface
func (ChangeMerged) Message(e gerritssh.Event, _ project.Config, _ *gerrit.Client, me MessageEnricher) (Message, error) {
	// we let the owner know their change was merged
	var m Message
	m.Fallback = fmt.Sprintf("%s: merged %s: %s",
		e.Change.Owner.Name,
		e.Change.URL,
		e.Change.Subject,
	)
	m.Pretext = DefaultPretext("Merged", e)
	m.Fields = []MessageField{OwnerField(e, me), ProjectField(e)}
	return m, nil
}
