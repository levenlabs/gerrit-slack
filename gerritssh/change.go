package gerritssh

import (
	"fmt"
	"net/url"
)

// ChangeStatus describes the current status of the change
type ChangeStatus string

const (
	// ChangeStatusNew means change is still being reviewed
	ChangeStatusNew ChangeStatus = "NEW"

	// ChangeStatusMerged means change has been merged to its branch
	ChangeStatusMerged ChangeStatus = "MERGED"

	// ChangeStatusAbandoned means change was abandoned by its owner or administrator
	ChangeStatusAbandoned ChangeStatus = "ABANDONED"
)

// PatchSetKind describes the type of patch set
type PatchSetKind string

const (
	// PatchSetKindRework is nontrivial content changes
	PatchSetKindRework PatchSetKind = "REWORK"

	// PatchSetKindTrivialRebase is conflict-free merge between the new parent and the
	// prior patch set
	PatchSetKindTrivialRebase PatchSetKind = "TRIVIAL_REBASE"

	// PatchSetKindMergeFirstParentUpdate is conflict-free change of first (left) parent
	// of a merge commit
	PatchSetKindMergeFirstParentUpdate PatchSetKind = "MERGE_FIRST_PARENT_UPDATE"

	// PatchSetKindNoCodeChange is no code changed; same tree and same parent tree
	PatchSetKindNoCodeChange PatchSetKind = "NO_CODE_CHANGE"

	// PatchSetKindNoChange is no changes; same commit message, same tree and same parent
	// tree
	PatchSetKindNoChange PatchSetKind = "NO_CHANGE"
)

// ChangeIDWithProjectNumber formats the given project/number into a Change's ID
func ChangeIDWithProjectNumber(project string, number int64) string {
	return fmt.Sprintf("%s~%d", url.PathEscape(project), number)
}
