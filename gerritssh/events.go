package gerritssh

import (
	"bufio"
	"context"
	"encoding/json"

	"golang.org/x/crypto/ssh"

	"github.com/levenlabs/go-llog"
)

const (
	// EventTypeAssigneeChanged is sent when the assignee of a change has been
	// modified
	EventTypeAssigneeChanged = "assignee-changed"

	// EventTypeChangeAbandoned is sent when a change has been abandoned
	EventTypeChangeAbandoned = "change-abandoned"

	// EventTypeChangeMerged is sent when a change has been merged into the git
	// repository
	EventTypeChangeMerged = "change-merged"

	// EventTypeChangeRestored is sent when an abandoned change has been restored
	EventTypeChangeRestored = "change-restored"

	// EventTypeCommentAdded is sent when a review comment has been posted on
	// a change
	EventTypeCommentAdded = "comment-added"

	// EventTypeDroppedOutput is sent to notify a client that events have been
	// dropped
	EventTypeDroppedOutput = "dropped-output"

	// EventTypeHashtagsChanged is sent when the hashtags have been added to or
	// removed from a change
	EventTypeHashtagsChanged = "hashtags-changed"

	// EventTypeProjectCreated is sent when a new project has been created
	EventTypeProjectCreated = "project-created"

	// EventTypePatchSetCreated is sent when a new change has been uploaded, or
	// a new patch set has been uploaded to an existing change
	EventTypePatchSetCreated = "patchset-created"

	// EventTypeRefUpdated is sent when a reference is updated in a git repository
	EventTypeRefUpdated = "ref-updated"

	// EventTypeReviewerAdded is sent when a reviewer is added to a change
	EventTypeReviewerAdded = "reviewer-added"

	// EventTypeReviewerDeleted is sent when a reviewer (with a vote) is removed
	// from a change
	EventTypeReviewerDeleted = "reviewer-deleted"

	// EventTypeTopicChanged is sent when the topic of a change has been changed
	EventTypeTopicChanged = "topic-changed"

	// EventTypeWorkInProgressStateChanged is sent when the the WIP state of the
	// change has changed
	EventTypeWorkInProgressStateChanged = "wip-state-changed"

	// EventTypePrivateStateChanged is sent when the the private state of the
	// change has changed
	EventTypePrivateStateChanged = "private-state-changed"

	// EventTypeVoteDeleted is sent when a vote was removed from a change
	EventTypeVoteDeleted = "vote-deleted"

	// EventTypeRefReplicationScheduled is sent when replication is scheduled for a ref
	EventTypeRefReplicationScheduled = "ref-replication-scheduled"

	// EventTypeRefReplicated is sent when a ref has been replicated
	EventTypeRefReplicated = "ref-replicated"

	// EventTypeRefReplicationDone is sent when replication is done for a ref
	EventTypeRefReplicationDone = "ref-replication-done"
)

// Event describes a major event that occured in the gerrit server
// from https://gerrit-review.googlesource.com/Documentation/cmd-stream-events.html
// structures from https://gerrit-review.googlesource.com/Documentation/json.html
type Event struct {
	Type string `json:"type"`

	Change    EventChange    `json:"change"`
	PatchSet  EventPatchSet  `json:"patchSet"`
	RefUpdate EventRefUpdate `json:"refUpdate"`

	Author    EventAccount `json:"author"`
	Submitter EventAccount `json:"submitter"`
	Reviewer  EventAccount `json:"reviewer"`
	Remover   EventAccount `json:"remover"`
	Changer   EventAccount `json:"changer"`
	Uploader  EventAccount `json:"uploader"`
	Editor    EventAccount `json:"editor"`
	Abandoner EventAccount `json:"abandoner"`
	Restorer  EventAccount `json:"restorer"`

	Approvals   []EventApproval `json:"approvals"`
	Added       []string        `json:"added"`
	Removed     []string        `json:"removed"`
	Hashtags    []string        `json:"hashtags"`
	ProjectName string          `json:"projectName"`
	ProjectHead string          `json:"projectHead"`
	OldTopic    string          `json:"oldTopic"`
	Comment     string          `json:"comment"`
	Reason      string          `json:"reason"`
	NewRevision string          `json:"newRev"`
	OldAssignee EventAccount    `json:"oldAssignee"`
	TargetNode  string          `json:"targetNode"`
	Status      string          `json:"status"`
	RefStatus   string          `json:"refStatus"`
	NodesCount  int64           `json:"nodesCount"`

	TSCreated int64 `json:"eventCreatedOn"`
}

// KV returns a KV for the given event
func (e Event) KV() llog.KV {
	var project string
	if e.Change.Project != "" {
		project = e.Change.Project
	} else if e.ProjectName != "" {
		project = e.ProjectName
	}
	return llog.KV{
		"type":    e.Type,
		"project": project,
	}
}

// EventChange describes a change inside an Event
type EventChange struct {
	Project       string       `json:"project"`
	Branch        string       `json:"branch"`
	Topic         string       `json:"topic"`
	ChangeID      string       `json:"id"`
	Number        int64        `json:"number"`
	Subject       string       `json:"subject"`
	Owner         EventAccount `json:"owner"`
	URL           string       `json:"url"`
	CommitMessage string       `json:"commitMessage"`
	Status        ChangeStatus `json:"status"`
	Open          bool         `json:"open"`
	Private       bool         `json:"private"`
	WIP           bool         `json:"wip"`
	TSCreated     int64        `json:"createdOn"`
}

// EventPatchSet describes a patch set inside an Event
type EventPatchSet struct {
	Number         int64        `json:"number"`
	Revision       string       `json:"revision"`
	Parents        []string     `json:"parents"`
	Ref            string       `json:"ref"`
	Uploader       EventAccount `json:"uploader"`
	Kind           PatchSetKind `json:"kind"`
	Author         EventAccount `json:"author"`
	SizeInsertions int64        `json:"sizeInsertions"`
	SizeDeletions  int64        `json:"sizeDeletions"`
	TSCreated      int64        `json:"createdOn"`
}

// EventAccount describes a user account inside an Event
type EventAccount struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// EventRefUpdate describes a ref inside an Event
type EventRefUpdate struct {
	OldRevision string `json:"oldRev"`
	// NewRevision, if 0000000000000000000000000000000000000000, means it was
	// deleted
	NewRevision string `json:"newRev"`
	RefName     string `json:"refName"`
	Project     string `json:"project"`
}

// EventApproval describes an approval inside an Event
type EventApproval struct {
	Type        string       `json:"type"`
	Description string       `json:"description"`
	Value       string       `json:"value"`
	OldValue    string       `json:"oldValue"`
	By          EventAccount `json:"by"`
}

// StreamEvents will start listening for real-time gerrit events
func (e *Client) StreamEvents(ctx context.Context, ch chan Event) error {
	sess, err := e.Dial()
	if err != nil {
		return err
	}
	sout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	sos := bufio.NewScanner(sout)
	runCh := make(chan error, 1)

	// start running stream-events and wait for it to disconnect
	go func() {
		// Run calls Start and then Wait
		runCh <- sess.Run("gerrit stream-events")
	}()

	readCh := make(chan error, 1)
	// listen on the stdout of ssh session and send events to ch
	go func() {
		for sos.Scan() {
			var ev Event
			if err := json.Unmarshal(sos.Bytes(), &ev); err != nil {
				llog.Error("error unmarshalling event", llog.ErrKV(err))
				continue
			}
			llog.Info("gerrit event", ev.KV())
			ch <- ev
		}
		readCh <- sos.Err()
	}()

	select {
	case <-ctx.Done():
	case err = <-runCh:
		close(runCh)
	case err = <-readCh:
		close(readCh)
	}
	sess.Close()
	// now wait for both goroutines to stop
	<-runCh
	<-readCh
	// ensure there's some error that's returned
	if err == nil {
		err = &ssh.ExitMissingError{}
	}
	return err
}
