package events

import (
	"fmt"
	"strings"

	"github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/gerrit-slack/gerritssh"
	"github.com/levenlabs/gerrit-slack/project"
)

func init() {
	var h CommentAdded
	register(h.Type(), h)
}

// CommentAdded handles the comment-added event
type CommentAdded struct{}

// Type implements the EventHandler interface
func (CommentAdded) Type() string {
	return gerritssh.EventTypeCommentAdded
}

// Ignore implements the EventHandler interface
func (CommentAdded) Ignore(e gerritssh.Event, pcfg project.Config) (bool, error) {
	if !pcfg.PublishOnCommentAdded {
		return true, nil
	}
	ignore, err := regexMatch(pcfg.IgnoreAuthors, e.Author.Username)
	if err != nil {
		return false, err
	}
	if ignore {
		return true, nil
	}
	// if the comment contains 2 new-lines then there was a comment WITH the votes
	// so there's no reason to check votes
	if len(e.Approvals) == 0 || strings.Contains(e.Comment, "\n\n") {
		return false, nil
	}
	var voted bool
	// TODO: remove this once https://bugs.chromium.org/p/gerrit/issues/detail?id=8494
	for _, v := range e.Approvals {
		if v.OldValue != "" {
			voted = true
			ignore, err = regexMatch(pcfg.IgnoreOnlyLabels, v.Type)
			if err != nil {
				return false, err
			}
			// if we shouldn't ignore this label then immediately bail
			if !ignore {
				return false, nil
			}
		}
	}
	// if we found at least one vote then we should ignore because that means that
	// IgnoreOnlyLabels matched all of the voted labels
	if voted {
		return true, nil
	}
	return false, nil
}

// Message implements the EventHandler interface
func (CommentAdded) Message(e gerritssh.Event, _ project.Config, c *gerrit.Client) (Message, error) {
	var m Message
	var voted bool
	if len(e.Approvals) > 0 {
		// TODO: remove this once https://bugs.chromium.org/p/gerrit/issues/detail?id=8494
		for _, v := range e.Approvals {
			if v.OldValue != "" {
				voted = true
				break
			}
		}
	}
	action := "commented on"
	if voted {
		action = "voted on"
	}
	m.Fallback = fmt.Sprintf("%s %s %s: %s",
		e.Author.Name,
		action,
		e.Change.URL,
		e.Change.Subject,
	)
	action = fmt.Sprintf("%s %s", e.Author.Name, action)
	m.Pretext = DefaultPretext(action, e)

	m.Fields = []MessageField{
		OwnerField(e),
	}
	// if the author is the owner, then let reviewers know
	if e.Author.Email == e.Change.Owner.Email {
		// get the list of reviewers for the reviewers field
		rs, _, err := c.Changes.ListReviewers(gerritssh.ChangeIDWithProjectNumber(e.Change.Project, e.Change.Number))
		if err != nil {
			return m, err
		}
		m.Fields = append(m.Fields, ReviewersField(e, *rs))
	}
	m.Text = e.Comment
	return m, nil
}
