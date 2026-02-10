package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/identity"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
	"github.com/lydakis/jul/cli/internal/policy"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

func resolvePromoteStrategy(explicit, policyStrategy string, policyOK bool) (string, error) {
	strategy := strings.TrimSpace(explicit)
	if strategy == "" && policyOK {
		strategy = strings.TrimSpace(policyStrategy)
	}
	if strategy == "" {
		strategy = "rebase"
	}
	strategy = strings.ToLower(strategy)
	switch strategy {
	case "rebase", "squash", "merge":
		return strategy, nil
	default:
		return "", promoteError{
			Code:    "promote_invalid_strategy",
			Message: fmt.Sprintf("unsupported promote strategy %q", strategy),
		}
	}
}

func ensureWorkspaceLeaseCurrent(repoRoot, user, workspace string) error {
	remote, err := remotesel.Resolve()
	if err != nil {
		switch err {
		case remotesel.ErrNoRemote, remotesel.ErrRemoteMissing:
			return nil
		default:
			return err
		}
	}
	if strings.TrimSpace(remote.Name) == "" {
		return nil
	}
	_, _ = identity.ResolveUserNamespace(remote.Name)

	workspaceRefName := workspaceRef(user, workspace)
	remoteTip, err := remoteRefTip(remote.Name, workspaceRefName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(remoteTip) == "" {
		return nil
	}
	if err := fetchRef(remote.Name, workspaceRefName); err != nil {
		return err
	}

	leaseSHA, err := readWorkspaceLease(repoRoot, workspace)
	if err != nil || strings.TrimSpace(leaseSHA) == "" {
		return promoteError{
			Code:    "promote_workspace_lease_missing",
			Message: "promote blocked: workspace lease missing; run 'jul ws checkout @' first",
		}
	}

	leaseBase := normalizeBaseCommit(leaseSHA)
	remoteBase := normalizeBaseCommit(remoteTip)
	if leaseBase == "" || remoteBase == "" {
		return nil
	}
	if strings.TrimSpace(leaseBase) == strings.TrimSpace(remoteBase) {
		return nil
	}
	if gitutil.IsAncestor(leaseBase, remoteBase) {
		return promoteError{
			Code:    "promote_base_advanced",
			Message: "promote blocked: base advanced; run 'jul ws restack' or 'jul ws checkout'",
		}
	}
	return promoteError{
		Code:    "promote_base_diverged",
		Message: "promote blocked: workspace base diverged; run 'jul ws checkout' to realign",
	}
}

func normalizeBaseCommit(sha string) string {
	trimmed := strings.TrimSpace(sha)
	if trimmed == "" {
		return ""
	}
	msg, err := gitutil.CommitMessage(trimmed)
	if err == nil && isDraftMessage(msg) {
		if parent, err := gitutil.ParentOf(trimmed); err == nil {
			if parent = strings.TrimSpace(parent); parent != "" {
				return parent
			}
		}
	}
	return trimmed
}

func readWorkspaceLease(repoRoot, workspace string) (string, error) {
	path := filepath.Join(repoRoot, ".jul", "workspaces", workspace, "lease")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func enforcePromotePolicy(cfg policy.PromotePolicy, checkpointSHA, changeID string) error {
	if cfg.MinCoveragePct == nil && len(cfg.RequiredChecks) == 0 && cfg.RequireSuggestionsAddressed == nil {
		return nil
	}
	view, err := resolveAttestationView(checkpointSHA)
	if err != nil {
		return err
	}
	if view.Stale {
		return promoteError{
			Code:    "promote_policy_failed",
			Message: "promote blocked: CI results are stale; rerun CI on the latest checkpoint",
			Next: []output.NextAction{
				{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
			},
		}
	}
	att := view.Attestation
	if att == nil {
		return promoteError{
			Code:    "promote_policy_failed",
			Message: "promote blocked: no CI results found for latest checkpoint",
			Next: []output.NextAction{
				{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
			},
		}
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return err
	}
	if strings.TrimSpace(att.DeviceID) == "" || strings.TrimSpace(att.DeviceID) != strings.TrimSpace(deviceID) {
		return promoteError{
			Code:    "promote_policy_failed",
			Message: "promote blocked: CI results were not computed locally on this device; rerun CI on the latest checkpoint",
			Next: []output.NextAction{
				{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
			},
		}
	}

	for _, check := range cfg.RequiredChecks {
		name := strings.ToLower(strings.TrimSpace(check))
		if name == "" {
			continue
		}
		switch name {
		case "test", "tests":
			if !isPassingStatus(att.TestStatus) {
				return promoteError{
					Code:    "promote_policy_failed",
					Message: fmt.Sprintf("promote blocked: test status %s", statusLabel(att.TestStatus)),
					Next: []output.NextAction{
						{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
					},
				}
			}
		case "compile", "build":
			if !isPassingStatus(att.CompileStatus) {
				return promoteError{
					Code:    "promote_policy_failed",
					Message: fmt.Sprintf("promote blocked: compile status %s", statusLabel(att.CompileStatus)),
					Next: []output.NextAction{
						{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
					},
				}
			}
		default:
			if !isPassingStatus(att.Status) {
				return promoteError{
					Code:    "promote_policy_failed",
					Message: fmt.Sprintf("promote blocked: CI status %s", statusLabel(att.Status)),
					Next: []output.NextAction{
						{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
					},
				}
			}
		}
	}

	if cfg.MinCoveragePct != nil {
		coverage := att.CoverageLinePct
		if coverage == nil {
			coverage = att.CoverageBranchPct
		}
		if coverage == nil {
			return promoteError{
				Code:    "promote_policy_failed",
				Message: "promote blocked: coverage data missing for latest checkpoint",
				Next: []output.NextAction{
					{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
				},
			}
		}
		if *coverage < *cfg.MinCoveragePct {
			return promoteError{
				Code:    "promote_policy_failed",
				Message: fmt.Sprintf("promote blocked: coverage %.1f%% below policy threshold %.1f%%", *coverage, *cfg.MinCoveragePct),
				Next: []output.NextAction{
					{Action: "rerun", Command: fmt.Sprintf("jul ci run --target %s --json", checkpointSHA)},
					{Action: "bypass", Command: "jul promote --no-policy --json"},
				},
			}
		}
	}

	if cfg.RequireSuggestionsAddressed != nil && *cfg.RequireSuggestionsAddressed && strings.TrimSpace(changeID) != "" {
		if pending, _ := metadata.ListSuggestions(changeID, "pending", 1); len(pending) > 0 {
			return promoteError{
				Code:    "promote_policy_failed",
				Message: "promote blocked: pending suggestions must be addressed",
			}
		}
	}

	return nil
}

func isPassingStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "ok", "success", "succeeded":
		return true
	default:
		return false
	}
}

func statusLabel(status string) string {
	if strings.TrimSpace(status) == "" {
		return "missing"
	}
	return strings.TrimSpace(status)
}

func resolvePromoteBaseTip(remoteTip, localTip string, checkpoints []metadata.ChangeCheckpoint) string {
	if strings.TrimSpace(remoteTip) != "" {
		return strings.TrimSpace(remoteTip)
	}
	if strings.TrimSpace(localTip) != "" {
		return strings.TrimSpace(localTip)
	}
	if len(checkpoints) == 0 {
		return ""
	}
	parent, _ := gitutil.ParentOf(checkpoints[0].SHA)
	return strings.TrimSpace(parent)
}

func promoteRebase(repoRoot, baseTip string, checkpoints []string) ([]string, error) {
	if strings.TrimSpace(baseTip) == "" || len(checkpoints) == 0 {
		return nil, fmt.Errorf("rebase base and checkpoints required")
	}
	worktree, cleanup, err := createPromoteWorktree(repoRoot, baseTip)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	published := make([]string, 0, len(checkpoints))
	for _, sha := range checkpoints {
		if _, err := gitutil.Git("-C", worktree, "cherry-pick", "--allow-empty", sha); err != nil {
			_, _ = gitutil.Git("-C", worktree, "cherry-pick", "--abort")
			if strings.Contains(strings.ToLower(err.Error()), "conflict") {
				return nil, promoteError{
					Code:    "promote_rebase_conflict",
					Message: "promote rebase conflict; resolve and retry",
					Next: []output.NextAction{
						{Action: "merge", Command: "jul merge --json"},
					},
				}
			}
			return nil, err
		}
		head, err := gitutil.Git("-C", worktree, "rev-parse", "HEAD")
		if err != nil {
			return nil, err
		}
		published = append(published, strings.TrimSpace(head))
	}
	return published, nil
}

func promoteSquash(repoRoot, baseTip string, checkpoints []string, message string) (string, error) {
	if strings.TrimSpace(baseTip) == "" || len(checkpoints) == 0 {
		return "", fmt.Errorf("squash base and checkpoints required")
	}
	worktree, cleanup, err := createPromoteWorktree(repoRoot, baseTip)
	if err != nil {
		return "", err
	}
	defer cleanup()

	for _, sha := range checkpoints {
		if _, err := gitutil.Git("-C", worktree, "cherry-pick", "--allow-empty", "--no-commit", sha); err != nil {
			_, _ = gitutil.Git("-C", worktree, "cherry-pick", "--abort")
			if strings.Contains(strings.ToLower(err.Error()), "conflict") {
				return "", promoteError{
					Code:    "promote_squash_conflict",
					Message: "promote squash conflict; resolve and retry",
					Next: []output.NextAction{
						{Action: "merge", Command: "jul merge --json"},
					},
				}
			}
			return "", err
		}
	}
	if err := gitCommitWithMessage(worktree, message); err != nil {
		return "", err
	}
	head, err := gitutil.Git("-C", worktree, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
}

func promoteMerge(repoRoot, baseTip, mergeSHA, message string) (string, error) {
	if strings.TrimSpace(baseTip) == "" || strings.TrimSpace(mergeSHA) == "" {
		return "", fmt.Errorf("merge base and checkpoint required")
	}
	worktree, cleanup, err := createPromoteWorktree(repoRoot, baseTip)
	if err != nil {
		return "", err
	}
	defer cleanup()

	if _, err := gitutil.Git("-C", worktree, "merge", "--no-ff", "--no-commit", mergeSHA); err != nil {
		_, _ = gitutil.Git("-C", worktree, "merge", "--abort")
		if strings.Contains(strings.ToLower(err.Error()), "conflict") {
			return "", promoteError{
				Code:    "promote_merge_conflict",
				Message: "promote merge conflict; resolve and retry",
				Next: []output.NextAction{
					{Action: "merge", Command: "jul merge --json"},
				},
			}
		}
		return "", err
	}
	if err := gitCommitWithMessage(worktree, message); err != nil {
		return "", err
	}
	head, err := gitutil.Git("-C", worktree, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
}

func writePromoteChangeIDNotes(changeID string, eventID int, strategy string, checkpoints []metadata.ChangeCheckpoint, published []string) error {
	if strings.TrimSpace(changeID) == "" || len(published) == 0 {
		return nil
	}
	strategy = strings.TrimSpace(strategy)
	if strategy == "" {
		strategy = "rebase"
	}
	switch strategy {
	case "rebase":
		for i, publishedSHA := range published {
			cpSHA := ""
			if i < len(checkpoints) {
				cpSHA = strings.TrimSpace(checkpoints[i].SHA)
			}
			base, head := traceAnchorsForCommit(cpSHA)
			note := metadata.ChangeIDNote{
				ChangeID:            changeID,
				PromoteEventID:      eventID,
				Strategy:            strategy,
				SourceCheckpointSHA: cpSHA,
				TraceBase:           base,
				TraceHead:           head,
			}
			if err := metadata.WriteChangeIDNote(publishedSHA, note); err != nil {
				return err
			}
		}
	default:
		traceBase, traceHead := traceRange(checkpoints)
		checkpointSHAs := make([]string, 0, len(checkpoints))
		for _, cp := range checkpoints {
			checkpointSHAs = append(checkpointSHAs, strings.TrimSpace(cp.SHA))
		}
		for _, publishedSHA := range published {
			note := metadata.ChangeIDNote{
				ChangeID:       changeID,
				PromoteEventID: eventID,
				Strategy:       strategy,
				CheckpointSHAs: checkpointSHAs,
				TraceBase:      traceBase,
				TraceHead:      traceHead,
			}
			if err := metadata.WriteChangeIDNote(publishedSHA, note); err != nil {
				return err
			}
		}
	}
	return nil
}

func traceAnchorsForCommit(sha string) (string, string) {
	if strings.TrimSpace(sha) == "" {
		return "", ""
	}
	msg, err := gitutil.CommitMessage(sha)
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(gitutil.ExtractTraceBase(msg)), strings.TrimSpace(gitutil.ExtractTraceHead(msg))
}

func traceRange(checkpoints []metadata.ChangeCheckpoint) (string, string) {
	if len(checkpoints) == 0 {
		return "", ""
	}
	baseMsg, _ := gitutil.CommitMessage(checkpoints[0].SHA)
	headMsg, _ := gitutil.CommitMessage(checkpoints[len(checkpoints)-1].SHA)
	return strings.TrimSpace(gitutil.ExtractTraceBase(baseMsg)), strings.TrimSpace(gitutil.ExtractTraceHead(headMsg))
}

func ensureChangeID(message, changeID string) string {
	if strings.TrimSpace(changeID) == "" {
		return message
	}
	if gitutil.ExtractChangeID(message) != "" {
		return message
	}
	return strings.TrimSpace(message) + "\n\nChange-Id: " + strings.TrimSpace(changeID) + "\n"
}

func createPromoteWorktree(repoRoot, baseSHA string) (string, func(), error) {
	dir, err := os.MkdirTemp(filepath.Join(repoRoot, ".jul"), "promote-worktree-")
	if err != nil {
		return "", nil, err
	}
	if _, err := gitutil.Git("-C", repoRoot, "worktree", "add", "--detach", dir, baseSHA); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	cleanup := func() {
		_, _ = gitutil.Git("-C", repoRoot, "worktree", "remove", "--force", dir)
		_ = os.RemoveAll(dir)
		_, _ = gitutil.Git("-C", repoRoot, "worktree", "prune")
	}
	return dir, cleanup, nil
}

func gitCommitWithMessage(dir, message string) error {
	cmd := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-F", "-")
	cmd.Stdin = strings.NewReader(message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
