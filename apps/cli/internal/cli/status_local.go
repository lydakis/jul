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
	if att == nil && draftSHA != "" {
		att, _ = metadata.GetAttestation(draftSHA)
	}
	if att == nil {
		att, _ = metadata.GetAttestation(info.SHA)
	}

	draftChangeID := ""
	if draftSHA != "" {
		if msg, err := gitutil.CommitMessage(draftSHA); err == nil {
			draftChangeID = gitutil.ExtractChangeID(msg)
		}
		if draftChangeID == "" {
			draftChangeID = gitutil.FallbackChangeID(draftSHA)
		}
	}

	changeID := info.ChangeID
	if checkpoint != nil && checkpoint.ChangeID != "" {
		changeID = checkpoint.ChangeID
	} else if draftChangeID != "" {
		changeID = draftChangeID
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
	if tree, err := readWorkingTreeStatus(); err == nil {
		status.WorkingTree = tree
	}

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
	configured := hasCIConfig()
	if !configured {
		if root, err := gitutil.RepoTopLevel(); err == nil {
			configured = hasCIInference(root)
		}
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
	if completed != nil && hasRealCIResult(completed.Result.Commands) {
		configured = true
	}
	if running != nil && running.CommitSHA == draftSHA {
		configured = true
	}
	if !configured && !resultsCurrent && (running == nil || running.CommitSHA != draftSHA) {
		return nil
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

func hasCIConfig() bool {
	cfg, ok, err := cicmd.LoadConfig()
	if err != nil || !ok || len(cfg.Commands) == 0 {
		return false
	}
	for _, cmd := range cfg.Commands {
		if isRealCICommand(cmd.Command) {
			return true
		}
	}
	return false
}

func hasCIInference(root string) bool {
	cmds := cicmd.InferDefaultCommands(root)
	for _, cmd := range cmds {
		if isRealCICommand(cmd) {
			return true
		}
	}
	return false
}

func hasRealCIResult(cmds []cicmd.CommandResult) bool {
	for _, cmd := range cmds {
		if isRealCICommand(cmd.Command) {
			return true
		}
	}
	return false
}

func isRealCICommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return false
	}
	if strings.EqualFold(trimmed, "true") {
		return false
	}
	return true
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

func readWorkingTreeStatus() (*output.WorkingTreeStatus, error) {
	out, err := gitutil.Git("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return &output.WorkingTreeStatus{Clean: true}, nil
	}

	status := &output.WorkingTreeStatus{}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		code := line[:2]
		path := strings.TrimSpace(line[2:])
		if path == "" {
			continue
		}
		if code == "??" {
			status.Untracked = append(status.Untracked, output.WorkingTreeEntry{
				Path:   path,
				Status: "?",
			})
			continue
		}
		staged := code[0]
		unstaged := code[1]
		if staged != ' ' {
			status.Staged = append(status.Staged, output.WorkingTreeEntry{
				Path:   path,
				Status: string(staged),
			})
		}
		if unstaged != ' ' && unstaged != '?' {
			status.Unstaged = append(status.Unstaged, output.WorkingTreeEntry{
				Path:   path,
				Status: string(unstaged),
			})
		}
	}
	if len(status.Staged) == 0 && len(status.Unstaged) == 0 && len(status.Untracked) == 0 {
		status.Clean = true
	}
	return status, nil
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
