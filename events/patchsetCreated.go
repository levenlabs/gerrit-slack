package events

import (
	"fmt"

	"github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/gerrit-slack/gerritssh"
	"github.com/levenlabs/gerrit-slack/project"
	llog "github.com/levenlabs/go-llog"
)

func init() {
	var h PatchSetCreated
	register(h.Type(), h)
}

// PatchSetCreated handles the patchset-created event
type PatchSetCreated struct{}

// Type implements the EventHandler interface
func (PatchSetCreated) Type() string {
	return gerritssh.EventTypePatchSetCreated
}

func unchangedPatchSetKind(k gerritssh.PatchSetKind) bool {
	switch k {
	case gerritssh.PatchSetKindTrivialRebase:
		return true
	case gerritssh.PatchSetKindMergeFirstParentUpdate:
		return true
	case gerritssh.PatchSetKindNoCodeChange:
		return true
	case gerritssh.PatchSetKindNoChange:
		return true
	case gerritssh.PatchSetKindRework:
		return false
	default:
		llog.Warn("unknown patch set kind", llog.KV{"kind": k})
	}
	// Default unknown kind to changed
	return false
}

// Ignore implements the EventHandler interface
func (PatchSetCreated) Ignore(e gerritssh.Event, pcfg project.Config) (bool, error) {
	if !pcfg.PublishOnPatchSetCreated {
		return true, nil
	}
	if pcfg.IgnoreUnchangedPatchSet && unchangedPatchSetKind(e.PatchSet.Kind) {
		return true, nil
	}
	m, err := regexMatch(pcfg.IgnoreCommitMessage, e.Change.CommitMessage)
	if err != nil || m {
		return m, err
	}
	return regexMatch(pcfg.IgnoreAuthors, e.Author.Username)
}

// Message implements the EventHandler interface
func (PatchSetCreated) Message(e gerritssh.Event, _ project.Config, c *gerrit.Client) (Message, error) {
	// we let the owner know their change was merged
	var m Message
	m.Fallback = fmt.Sprintf("%s proposed %s: %s",
		e.Submitter.Name,
		e.Change.URL,
		e.Change.Subject,
	)
	action := "Proposed"
	if e.PatchSet.Number > 1 {
		action = "Updated"
	}
	m.Pretext = DefaultPretext(action, e)

	// get the list of reviewers for the reviewers field
	rs, _, err := c.Changes.ListReviewers(gerritssh.ChangeIDWithProjectNumber(e.Change.Project, e.Change.Number))
	if err != nil {
		return m, err
	}
	m.Fields = []MessageField{
		OwnerField(e),
		MessageField{
			Title: "Size",
			Value: fmt.Sprintf("+%d, -%d",
				e.PatchSet.SizeInsertions,
				e.PatchSet.SizeDeletions,
			),
			Short: true,
		},
		ReviewersField(e, *rs),
	}
	return m, nil
}
