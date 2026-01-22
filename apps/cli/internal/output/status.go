package output

import (
	"fmt"
	"io"
	"strings"
)

type Status struct {
	WorkspaceID        string            `json:"workspace_id"`
	Repo               string            `json:"repo"`
	Branch             string            `json:"branch"`
	DraftSHA           string            `json:"draft_sha"`
	ChangeID           string            `json:"change_id"`
	SyncStatus         string            `json:"sync_status"`
	LastCheckpoint     *CheckpointStatus `json:"last_checkpoint,omitempty"`
	AttestationStatus  string            `json:"attestation_status,omitempty"`
	SuggestionsPending int               `json:"suggestions_pending"`
}

type CheckpointStatus struct {
	CommitSHA string `json:"commit_sha"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	When      string `json:"when"`
	ChangeID  string `json:"change_id"`
}

func RenderStatus(w io.Writer, status Status, opts Options) {
	width := 11
	prefix := linePrefix(opts)
	writeKVIcon(w, prefix, "Workspace", status.WorkspaceID, width)
	writeKVIcon(w, prefix, "Repo", status.Repo, width)
	writeKVIcon(w, prefix, "Branch", status.Branch, width)
	writeKVIcon(w, prefix, "Draft", status.DraftSHA, width)
	writeKVIcon(w, prefix, "Change", status.ChangeID, width)
	if status.LastCheckpoint != nil {
		line := status.LastCheckpoint.CommitSHA
		if msg := strings.TrimSpace(status.LastCheckpoint.Message); msg != "" {
			line = fmt.Sprintf("%s %q", line, msg)
		}
		writeKVIcon(w, prefix, "Checkpoint", line, width)
	}
	if status.AttestationStatus != "" {
		icon := statusIconColored(status.AttestationStatus, opts)
		writeKVIcon(w, prefix, "CI", icon+statusText(status.AttestationStatus, opts), width)
	}
	if status.SuggestionsPending > 0 {
		warn := statusIconColored("warning", opts)
		if warn == "" {
			warn = statusIcon("warning", opts)
		}
		writeKVIcon(w, prefix, "Suggestions", warn+fmt.Sprintf("%d pending", status.SuggestionsPending), width)
	}
	writeKVIcon(w, prefix, "Sync", statusText(status.SyncStatus, opts), width)
}
