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
	Footer    string         `json:"footer"`
	Fields    []MessageField `json:"fields"`
	TS        int64          `json:"ts"`
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

// DefaultPretext returns the default pretext with the given action
func DefaultPretext(action string, e gerritssh.Event) string {
	return fmt.Sprintf("%s <%s|%s change %d>",
		action,
		e.Change.URL,
		e.Change.Project,
		e.Change.Number,
	)
}

// OwnerField retuns a Owner field with their name
func OwnerField(e gerritssh.Event) MessageField {
	return MessageField{
		Title: "Owner",
		Value: e.Change.Owner.Name,
		Short: true,
	}
}

// ProjectField retuns a Project field with the name
func ProjectField(e gerritssh.Event) MessageField {
	return MessageField{
		Title: "Project",
		Value: e.Change.Project,
		Short: true,
	}
}

// ReviewersField retuns a Reviewers field with reviewers
func ReviewersField(e gerritssh.Event, rs []gerrit.ReviewerInfo) MessageField {
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
		reviewers = append(reviewers, r.Name)
	}
	return MessageField{
		Title: "Reviewers",
		Value: strings.Join(reviewers, ", "),
		Short: len(reviewers) < 2,
	}
}
