package events

import (
	"encoding/json"
	"fmt"
	"strings"

	gerrit "github.com/andygrunwald/go-gerrit"
	"github.com/levenlabs/gerrit-slack/gerritssh"
)

// MessageField is a slack field
type MessageField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// Attachment is a slack attachment
type Attachment struct {
	Fallback  string         `json:"fallback"`
	Pretext   string         `json:"pretext"`
	Title     string         `json:"title"`
	TitleLink string         `json:"title_link"`
	Text      string         `json:"text"`
	Color     string         `json:"color"`
	Fields    []MessageField `json:"fields"`
}

// Message is a single-attachment message
type Message struct {
	Attachment
	Channel string
}

// MarshalJSON implements the json.Marshaler interface
func (m Message) MarshalJSON() ([]byte, error) {
	msg := struct {
		Channel     string       `json:"channel"`
		Attachments []Attachment `json:"attachments"`
	}{
		Channel:     m.Channel,
		Attachments: []Attachment{m.Attachment},
	}
	return json.Marshal(msg)
}

// DefaultPretext returns the default title with the given action
func DefaultPretext(action string, e gerritssh.Event) string {
	return fmt.Sprintf(`%s %s patchset: <%s|%s>`,
		action,
		e.Change.Project,
		e.Change.URL,
		e.Change.Subject,
	)
}

// OwnerField returns a Owner field with their name
func OwnerField(e gerritssh.Event, me MessageEnricher) MessageField {
	return MessageField{
		Title: "Owner",
		Value: me.MentionUser(e.Change.Owner.Email, e.Change.Owner.Name),
		Short: true,
	}
}

// ProjectField returns a Project field with the name
func ProjectField(e gerritssh.Event) MessageField {
	return MessageField{
		Title: "Project",
		Value: e.Change.Project,
		Short: true,
	}
}

// ReviewersField returns a Reviewers field with reviewers
func ReviewersField(e gerritssh.Event, rs []gerrit.ReviewerInfo, me MessageEnricher) MessageField {
	reviewers := []string{}
	for _, r := range rs {
		// ignore bots
		if r.Email == "" || r.Name == "" {
			continue
		}
		// ignore the owner
		if r.Email == e.Change.Owner.Email {
			continue
		}
		reviewers = append(reviewers, me.MentionUser(r.Email, r.Name))
	}
	return MessageField{
		Title: "Reviewers",
		Value: strings.Join(reviewers, ", "),
		Short: len(reviewers) < 2,
	}
}
