package events

import (
	"fmt"
	"strings"
	"time"

	gerrit "github.com/andygrunwald/go-gerrit"
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
func (PatchSetCreated) Message(e gerritssh.Event, pcfg project.Config, c *gerrit.Client) (Message, error) {
	// we let the owner know their change was merged
	var m Message
	action := "Proposed"
	if e.PatchSet.Number > 1 {
		action = "Updated"
	}
	m.Fallback = fmt.Sprintf("%s %s %s: %s",
		e.Uploader.Name,
		action,
		e.Change.URL,
		e.Change.Subject,
	)
	action = fmt.Sprintf("%s %s", e.Uploader.Name, action)
	m.Pretext = DefaultPretext(action, e)

	if !pcfg.PublishPatchSetCreatedImmediately {
		time.Sleep(5 * time.Second)
	}

	// get the list of reviewers for the reviewers field
	rs, _, err := c.Changes.ListReviewers(gerritssh.ChangeIDWithProjectNumber(e.Change.Project, e.Change.Number))
	if err != nil {
		return m, err
	}
	// we must handle 0 or neagtive numbers
	dstr := fmt.Sprintf("%d", e.PatchSet.SizeDeletions)
	if !strings.HasPrefix(dstr, "-") {
		dstr = "-" + dstr
	}
	m.Fields = []MessageField{
		ReviewersField(e, *rs),
		MessageField{
			Title: "Size",
			Value: fmt.Sprintf("+%d, %s",
				e.PatchSet.SizeInsertions,
				dstr,
			),
			Short: true,
		},
	}
	return m, nil
}
