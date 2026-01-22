package cli

import (
	"path/filepath"
	"strings"
	"time"

	cicmd "github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

func buildLocalStatus() (output.Status, error) {
	info, err := gitutil.CurrentCommit()
	if err != nil {
		info, err = fallbackCommitInfo()
		if err != nil {
			return output.Status{}, err
		}
	}
	repoName := config.RepoName()
	if repoName != "" {
		info.RepoName = repoName
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	wsID := config.WorkspaceID()

	draftSHA := ""
	if ref, err := syncRef(user, workspace); err == nil && gitutil.RefExists(ref) {
		if sha, err := gitutil.ResolveRef(ref); err == nil {
			draftSHA = sha
		}
	}
	if draftSHA == "" {
		ref := workspaceRef(user, workspace)
		if gitutil.RefExists(ref) {
			if sha, err := gitutil.ResolveRef(ref); err == nil {
				draftSHA = sha
			}
		}
	}
	if draftSHA == "" {
		draftSHA = info.SHA
	}

	var checkpoint *output.CheckpointStatus
	var att *client.Attestation
	last, err := latestCheckpoint()
	if err != nil {
		return output.Status{}, err
	}
	if last != nil {
		checkpoint = &output.CheckpointStatus{
			CommitSHA: last.SHA,
			Message:   firstLine(last.Message),
			Author:    last.Author,
			When:      last.When.Format("2006-01-02 15:04:05"),
			ChangeID:  last.ChangeID,
		}
		att, _ = metadata.GetAttestation(last.SHA)
	}
	if att == nil {
		att, _ = metadata.GetAttestation(info.SHA)
	}

	changeID := info.ChangeID
	if checkpoint != nil && checkpoint.ChangeID != "" {
		changeID = checkpoint.ChangeID
	}
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(info.SHA)
	}

	suggestions, err := metadata.ListSuggestions(changeID, "pending", 1000)
	if err != nil {
		return output.Status{}, err
	}

	status := output.Status{
		WorkspaceID:        wsID,
		Workspace:          workspace,
		WorkspaceDefault:   workspace == config.WorkspaceName(),
		Repo:               info.RepoName,
		Branch:             info.Branch,
		DraftSHA:           draftSHA,
		ChangeID:           changeID,
		SyncStatus:         "local",
		LastCheckpoint:     checkpoint,
		SuggestionsPending: len(suggestions),
	}
	if att != nil {
		status.AttestationStatus = att.Status
	}

	filesChanged := draftFilesChanged(draftSHA)
	status.Draft = &output.DraftStatus{
		CommitSHA:    draftSHA,
		ChangeID:     changeID,
		FilesChanged: filesChanged,
	}
	status.DraftCI = buildDraftCIStatus(draftSHA)

	checkpoints, err := listCheckpoints()
	if err != nil {
		return output.Status{}, err
	}
	summaries := make([]output.CheckpointSummary, 0, len(checkpoints))
	for _, cp := range checkpoints {
		ciStatus := ""
		if att, _ := metadata.GetAttestation(cp.SHA); att != nil {
			ciStatus = att.Status
		}
		suggestions, _ := metadata.ListSuggestions(cp.ChangeID, "pending", 1000)
		summaries = append(summaries, output.CheckpointSummary{
			CommitSHA:          cp.SHA,
			Message:            firstLine(cp.Message),
			ChangeID:           cp.ChangeID,
			When:               cp.When.Format("2006-01-02 15:04:05"),
			CIStatus:           ciStatus,
			SuggestionsPending: len(suggestions),
		})
	}
	status.Checkpoints = summaries
	status.PromoteStatus = buildPromoteStatus(checkpoints)
	return status, nil
}

func buildDraftCIStatus(draftSHA string) *output.CIStatusDetails {
	completed, err := cicmd.ReadCompleted()
	if err != nil {
		return nil
	}
	running, _ := cicmd.ReadRunning()
	if completed == nil && running == nil {
		return nil
	}
	status := "unknown"
	resultsCurrent := false
	if completed != nil {
		resultsCurrent = completed.CommitSHA == draftSHA
		if resultsCurrent {
			status = completed.Result.Status
		} else {
			status = "stale"
		}
	}
	if running != nil && running.CommitSHA == draftSHA {
		status = "running"
	}
	details := &output.CIStatusDetails{
		Status:          status,
		CurrentDraftSHA: draftSHA,
		ResultsCurrent:  resultsCurrent,
	}
	if completed != nil {
		details.CompletedSHA = completed.CommitSHA
		if !completed.Result.StartedAt.IsZero() && !completed.Result.FinishedAt.IsZero() {
			details.DurationMs = completed.Result.FinishedAt.Sub(completed.Result.StartedAt).Milliseconds()
		}
		checks := make([]output.CICheck, 0, len(completed.Result.Commands))
		for _, cmd := range completed.Result.Commands {
			checks = append(checks, output.CICheck{
				Name:       output.LabelForCommand(cmd.Command),
				Status:     cmd.Status,
				DurationMs: cmd.DurationMs,
				Output:     cmd.OutputExcerpt,
			})
		}
		if completed.CoverageLinePct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_line",
				Status: "pass",
				Value:  *completed.CoverageLinePct,
			})
		}
		if completed.CoverageBranchPct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_branch",
				Status: "pass",
				Value:  *completed.CoverageBranchPct,
			})
		}
		details.Results = checks
	}
	if running != nil {
		details.RunningSHA = running.CommitSHA
	}
	return details
}

func buildPromoteStatus(checkpoints []checkpointInfo) *output.PromoteStatus {
	if len(checkpoints) == 0 {
		return nil
	}
	target := config.PromoteTarget()
	if target == "" {
		target = "main"
	}
	targetRef := "refs/heads/" + target
	if !gitutil.RefExists(targetRef) {
		return &output.PromoteStatus{
			Target:           target,
			Eligible:         false,
			CheckpointsAhead: len(checkpoints),
		}
	}
	ahead := 0
	for _, cp := range checkpoints {
		_, err := gitutil.Git("merge-base", "--is-ancestor", cp.SHA, targetRef)
		if err != nil {
			ahead++
		}
	}
	return &output.PromoteStatus{
		Target:           target,
		Eligible:         true,
		CheckpointsAhead: ahead,
	}
}

func fallbackCommitInfo() (gitutil.CommitInfo, error) {
	sha, err := currentDraftSHA()
	if err != nil || strings.TrimSpace(sha) == "" {
		return gitutil.CommitInfo{}, err
	}
	message, _ := gitutil.CommitMessage(sha)
	author, _ := gitutil.Git("log", "-1", "--format=%an", sha)
	committedISO, _ := gitutil.Git("log", "-1", "--format=%cI", sha)
	top, _ := gitutil.RepoTopLevel()

	committed := time.Now().UTC()
	if committedISO != "" {
		if parsed, err := time.Parse(time.RFC3339, committedISO); err == nil {
			committed = parsed
		}
	}

	changeID := gitutil.ExtractChangeID(message)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(sha)
	}

	repoName := ""
	if top != "" {
		repoName = filepath.Base(top)
	}

	return gitutil.CommitInfo{
		SHA:       strings.TrimSpace(sha),
		Author:    strings.TrimSpace(author),
		Message:   message,
		Committed: committed,
		RepoName:  repoName,
		ChangeID:  changeID,
		TopLevel:  top,
	}, nil
}
