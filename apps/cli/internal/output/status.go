package output

import (
	"fmt"
	"io"
	"strings"
)

type Status struct {
	WorkspaceID        string              `json:"workspace_id"`
	Workspace          string              `json:"workspace,omitempty"`
	WorkspaceDefault   bool                `json:"workspace_default,omitempty"`
	Repo               string              `json:"repo"`
	Branch             string              `json:"branch"`
	DraftSHA           string              `json:"draft_sha"`
	ChangeID           string              `json:"change_id"`
	SyncStatus         string              `json:"sync_status"`
	LastCheckpoint     *CheckpointStatus   `json:"last_checkpoint,omitempty"`
	AttestationStatus  string              `json:"attestation_status,omitempty"`
	SuggestionsPending int                 `json:"suggestions_pending"`
	Draft              *DraftStatus        `json:"draft,omitempty"`
	Checkpoints        []CheckpointSummary `json:"checkpoints,omitempty"`
	PromoteStatus      *PromoteStatus      `json:"promote_status,omitempty"`
}

type CheckpointStatus struct {
	CommitSHA string `json:"commit_sha"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	When      string `json:"when"`
	ChangeID  string `json:"change_id"`
}

type DraftStatus struct {
	CommitSHA    string `json:"commit_sha,omitempty"`
	ChangeID     string `json:"change_id,omitempty"`
	FilesChanged int    `json:"files_changed"`
}

type CheckpointSummary struct {
	CommitSHA          string `json:"commit_sha"`
	Message            string `json:"message"`
	ChangeID           string `json:"change_id,omitempty"`
	When               string `json:"when,omitempty"`
	CIStatus           string `json:"ci_status,omitempty"`
	SuggestionsPending int    `json:"suggestions_pending,omitempty"`
}

type PromoteStatus struct {
	Target           string `json:"target,omitempty"`
	Eligible         bool   `json:"eligible"`
	CheckpointsAhead int    `json:"checkpoints_ahead,omitempty"`
}

func RenderStatus(w io.Writer, status Status, opts Options) {
	workspace := status.Workspace
	if workspace == "" {
		workspace = status.WorkspaceID
	}
	if status.WorkspaceDefault {
		workspace = workspace + " (default)"
	}
	fmt.Fprintf(w, "Workspace: %s\n", workspace)

	draft := status.Draft
	if draft == nil {
		draft = &DraftStatus{
			CommitSHA: status.DraftSHA,
			ChangeID:  status.ChangeID,
		}
	}
	draftID := draft.ChangeID
	if draftID == "" {
		draftID = draft.CommitSHA
	}
	draftLine := shortID(draftID, 6)
	if draft.FilesChanged > 0 {
		draftLine = fmt.Sprintf("%s (%d files changed)", draftLine, draft.FilesChanged)
	} else if draft.FilesChanged == 0 {
		draftLine = fmt.Sprintf("%s (clean)", draftLine)
	}
	fmt.Fprintf(w, "Draft: %s\n", draftLine)
	fmt.Fprintln(w, "")

	if len(status.Checkpoints) > 0 {
		fmt.Fprintln(w, "Checkpoints (not yet promoted):")
		for _, cp := range status.Checkpoints {
			line := fmt.Sprintf("  %s %q", shortID(cp.CommitSHA, 6), strings.TrimSpace(cp.Message))
			if cp.CIStatus != "" {
				icon := statusIconColored(cp.CIStatus, opts)
				if icon == "" {
					icon = statusIcon(cp.CIStatus, opts)
				}
				line += fmt.Sprintf(" %sCI %s", icon, statusText(cp.CIStatus, opts))
			}
			fmt.Fprintln(w, line)
			if cp.SuggestionsPending > 0 {
				warn := statusIconColored("warning", opts)
				if warn == "" {
					warn = statusIcon("warning", opts)
				}
				fmt.Fprintf(w, "    └─ %s%d suggestion pending\n", warn, cp.SuggestionsPending)
			}
		}
		fmt.Fprintln(w, "")
	}

	if status.PromoteStatus != nil && status.PromoteStatus.Target != "" {
		statusLine := ""
		if !status.PromoteStatus.Eligible {
			statusLine = " (target not found)"
		} else if status.PromoteStatus.CheckpointsAhead > 0 {
			statusLine = fmt.Sprintf(" (%d checkpoints behind)", status.PromoteStatus.CheckpointsAhead)
		} else {
			statusLine = " (up to date)"
		}
		fmt.Fprintf(w, "Promote target: %s%s\n", status.PromoteStatus.Target, statusLine)
	}
}
