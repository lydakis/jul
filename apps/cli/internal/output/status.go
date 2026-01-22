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
	DraftCI            *CIStatusDetails    `json:"draft_ci,omitempty"`
	WorkingTree        *WorkingTreeStatus  `json:"working_tree,omitempty"`
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

type WorkingTreeStatus struct {
	Clean     bool               `json:"clean"`
	Staged    []WorkingTreeEntry `json:"staged,omitempty"`
	Unstaged  []WorkingTreeEntry `json:"unstaged,omitempty"`
	Untracked []WorkingTreeEntry `json:"untracked,omitempty"`
}

type WorkingTreeEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
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
	if strings.TrimSpace(status.Repo) != "" {
		fmt.Fprintf(w, "Repo: %s\n", status.Repo)
	}
	if strings.TrimSpace(status.Branch) != "" {
		fmt.Fprintf(w, "Branch: %s\n", status.Branch)
	}

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

	if status.DraftCI != nil {
		renderDraftCI(w, status.DraftCI, opts, draft.CommitSHA)
	}
	if status.WorkingTree != nil {
		renderWorkingTree(w, status.WorkingTree, opts)
	}
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

func renderDraftCI(w io.Writer, ci *CIStatusDetails, opts Options, draftSHA string) {
	if ci == nil {
		return
	}
	if ci.RunningSHA != "" && ci.RunningSHA == draftSHA {
		icon := statusIconColored("running", opts)
		if icon == "" {
			icon = "⚡ "
		}
		fmt.Fprintf(w, "  %sCI running for current draft...\n", icon)
	}
	if ci.CompletedSHA != "" && !ci.ResultsCurrent {
		warn := statusIconColored("warning", opts)
		if warn == "" {
			warn = statusIcon("warning", opts)
		}
		fmt.Fprintf(w, "  %sCI results for previous draft (%s)\n", warn, shortID(ci.CompletedSHA, 6))
	}
	if len(ci.Results) == 0 && ci.Status != "" && ci.Status != "unknown" {
		icon := statusIconColored(ci.Status, opts)
		if icon == "" {
			icon = statusIcon(ci.Status, opts)
		}
		fmt.Fprintf(w, "  %sCI %s\n", icon, statusText(ci.Status, opts))
	}
	for _, check := range ci.Results {
		icon := statusIconColored(check.Status, opts)
		if icon == "" {
			icon = statusIcon(check.Status, opts)
		}
		line := fmt.Sprintf("  %s%s", icon, check.Name)
		if check.Value > 0 {
			line += fmt.Sprintf(" (%.0f%%)", check.Value)
		}
		fmt.Fprintln(w, line)
	}
}

func renderWorkingTree(w io.Writer, tree *WorkingTreeStatus, opts Options) {
	if tree == nil {
		return
	}
	if tree.Clean {
		fmt.Fprintln(w, "Working tree: clean")
		return
	}
	fmt.Fprintln(w, "Working tree:")
	if len(tree.Staged) > 0 {
		fmt.Fprintf(w, "  staged (%d):\n", len(tree.Staged))
		for _, entry := range tree.Staged {
			fmt.Fprintf(w, "    %s\n", formatWorkingEntry(entry, "staged", opts))
		}
	}
	if len(tree.Unstaged) > 0 {
		fmt.Fprintf(w, "  unstaged (%d):\n", len(tree.Unstaged))
		for _, entry := range tree.Unstaged {
			fmt.Fprintf(w, "    %s\n", formatWorkingEntry(entry, "unstaged", opts))
		}
	}
	if len(tree.Untracked) > 0 {
		fmt.Fprintf(w, "  untracked (%d):\n", len(tree.Untracked))
		for _, entry := range tree.Untracked {
			fmt.Fprintf(w, "    %s\n", formatWorkingEntry(entry, "untracked", opts))
		}
	}
}

func formatWorkingEntry(entry WorkingTreeEntry, bucket string, opts Options) string {
	status := strings.TrimSpace(entry.Status)
	if status == "" {
		status = "?"
	}
	icon := "•"
	color := ansiGray
	switch bucket {
	case "staged":
		icon = "+"
		color = ansiGreen
	case "unstaged":
		icon = "~"
		color = ansiYellow
	case "untracked":
		icon = "?"
		color = ansiCyan
	}
	if opts.Emoji {
		icon = "●"
	}
	if opts.Color {
		icon = colorize(icon, color)
		status = colorize(status, color)
	}
	return fmt.Sprintf("%s %s %s", icon, status, entry.Path)
}
