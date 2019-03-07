package events

import (
	"fmt"

	"github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/gerrit-slack/gerritssh"
	"github.com/levenlabs/gerrit-slack/project"
)

func init() {
	var h ReviewerAdded
	register(h.Type(), h)
}

// ReviewerAdded handles the reviewer-added event
type ReviewerAdded struct{}

// Type implements the EventHandler interface
func (ReviewerAdded) Type() string {
	return gerritssh.EventTypeReviewerAdded
}

// Ignore implements the EventHandler interface
func (ReviewerAdded) Ignore(e gerritssh.Event, pcfg project.Config) (bool, error) {
	if !pcfg.PublishOnReviewerAdded {
		return true, nil
	}
	if !pcfg.PublishPatchSetReviewersAdded {
		// if the event and the patchset were created at the same time, the reviewers
		// were added with the patchset
		if e.PatchSet.TSCreated == e.TSCreated {
			return true, nil
		}
	}
	return false, nil
}

// Message implements the EventHandler interface
func (ReviewerAdded) Message(e gerritssh.Event, _ project.Config, _ *gerrit.Client) (Message, error) {
	// we let the owner know their change was merged
	var m Message
	m.Fallback = fmt.Sprintf("%s asked to review %s: %s",
		e.Reviewer.Name,
		e.Change.URL,
		e.Change.Subject,
	)
	m.Pretext = DefaultPretext("Review requested for", e)

	m.Fields = []MessageField{
		OwnerField(e),
		MessageField{
			Title: "Reviewer",
			Value: e.Reviewer.Name,
			Short: true,
		},
	}
	return m, nil
}
