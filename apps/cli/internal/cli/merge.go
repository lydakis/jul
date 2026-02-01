package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

func newMergeCommand() Command {
	return Command{
		Name:    "merge",
		Summary: "Resolve diverged workspace conflicts",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("merge")
			autoApply := fs.Bool("apply", false, "Apply merge resolution without prompting")
			_ = fs.Parse(args)

			res, err := runMerge(*autoApply)
			if err != nil {
				var conflictErr MergeConflictError
				if errors.As(err, &conflictErr) {
					if strings.TrimSpace(conflictErr.Reason) != "" {
						res.Merge.Reason = strings.TrimSpace(conflictErr.Reason)
					}
					if strings.TrimSpace(conflictErr.Worktree) != "" {
						res.Merge.Worktree = strings.TrimSpace(conflictErr.Worktree)
					}
					if *jsonOut {
						if code := writeJSON(res); code != 0 {
							return code
						}
					} else {
						output.RenderMerge(os.Stdout, res.Merge)
					}
					return 1
				}
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "merge_failed", fmt.Sprintf("merge failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "merge failed: %v\n", err)
				}
				return 1
			}

			if *jsonOut {
				return writeJSON(res)
			}

			output.RenderMerge(os.Stdout, res.Merge)
			return 0
		},
	}
}

func runMerge(autoApply bool) (output.MergeOutput, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return output.MergeOutput{}, err
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return output.MergeOutput{}, err
	}

	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)

	if !gitutil.RefExists(workspaceRef) || !gitutil.RefExists(syncRef) {
		return output.MergeOutput{}, fmt.Errorf("workspace or sync ref missing; run 'jul sync' first")
	}

	remoteName := ""
	workspaceRemote := ""
	if remote, rerr := remotesel.Resolve(); rerr == nil {
		remoteName = remote.Name
		_ = fetchRef(remote.Name, workspaceRef)
		if sha, err := gitutil.ResolveRef(workspaceRef); err == nil {
			workspaceRemote = sha
		}
	}

	oursSHA, err := gitutil.ResolveRef(syncRef)
	if err != nil {
		return output.MergeOutput{}, err
	}
	theirsSHA := workspaceRemote
	if theirsSHA == "" {
		theirsSHA, err = gitutil.ResolveRef(workspaceRef)
		if err != nil {
			return output.MergeOutput{}, err
		}
	}

	if strings.TrimSpace(oursSHA) == strings.TrimSpace(theirsSHA) {
		return output.MergeOutput{Merge: output.MergeSummary{Status: "up_to_date"}}, nil
	}

	mergeBase, err := gitutil.MergeBase(oursSHA, theirsSHA)
	if err != nil || strings.TrimSpace(mergeBase) == "" {
		return output.MergeOutput{}, fmt.Errorf("failed to resolve merge base")
	}

	if draftParentMismatch(oursSHA, mergeBase) {
		return output.MergeOutput{}, fmt.Errorf("checkpoint base diverged; run 'jul ws checkout @' to reset")
	}
	if draftParentMismatch(theirsSHA, mergeBase) {
		return output.MergeOutput{}, fmt.Errorf("checkpoint base diverged; run 'jul ws checkout @' to reset")
	}

	worktree, err := agent.EnsureWorktree(repoRoot, oursSHA, agent.WorktreeOptions{AllowMergeInProgress: true})
	if err != nil {
		return output.MergeOutput{}, err
	}
	mergeInProgress := agent.MergeInProgress(worktree)
	dirtyWorktree, _ := worktreeDirty(worktree)
	if mergeInProgress {
		mergeHead, _ := gitOutputDir(worktree, "rev-parse", "-q", "--verify", "MERGE_HEAD")
		headSHA, _ := gitOutputDir(worktree, "rev-parse", "-q", "--verify", "HEAD")
		if strings.TrimSpace(mergeHead) != strings.TrimSpace(theirsSHA) || strings.TrimSpace(headSHA) != strings.TrimSpace(oursSHA) {
			mergeInProgress = false
		}
	}
	if !mergeInProgress && !dirtyWorktree {
		_ = gitDir(worktree, nil, "merge", "--abort")
		if err := gitDir(worktree, nil, "reset", "--hard", oursSHA); err != nil {
			return output.MergeOutput{}, err
		}
		if err := gitDir(worktree, nil, "clean", "-fd"); err != nil {
			return output.MergeOutput{}, err
		}

		mergeOutput, mergeErr := gitOutputDirAllowErr(worktree, "merge", "--no-commit", "--no-ff", theirsSHA)
		conflicts := mergeConflictFiles(worktree)
		if mergeErr != nil && len(conflicts) == 0 {
			return output.MergeOutput{}, fmt.Errorf("merge failed: %s", strings.TrimSpace(mergeOutput))
		}
	}

	conflicts := mergeConflictFiles(worktree)
	if mergeInProgress && len(conflicts) > 0 && !dirtyWorktree {
		out := output.MergeOutput{Merge: output.MergeSummary{Status: "conflicts", Conflicts: conflicts}}
		return out, MergeConflictError{Worktree: worktree, Conflicts: conflicts}
	}

	baseTarget := strings.TrimSpace(mergeBase)
	theirsMsg, _ := gitutil.CommitMessage(theirsSHA)
	if !isDraftMessage(theirsMsg) {
		baseTarget = strings.TrimSpace(theirsSHA)
	}
	changeID := changeIDFromCommit(mergeBase)
	if len(conflicts) > 0 && !dirtyWorktree {
		diff, _ := gitOutputDir(worktree, "diff")
		files := mergeConflictDetails(worktree, conflicts)
		req := agent.ReviewRequest{
			Version:       1,
			Action:        "resolve_conflict",
			WorkspacePath: worktree,
			Context: agent.ReviewContext{
				Checkpoint: mergeBase,
				ChangeID:   changeID,
				Diff:       diff,
				Files:      files,
				Conflicts:  conflicts,
			},
		}
		provider, err := agent.ResolveProvider()
		if err != nil {
			if errors.Is(err, agent.ErrAgentNotConfigured) || errors.Is(err, agent.ErrBundledMissing) {
				out := output.MergeOutput{Merge: output.MergeSummary{Status: "conflicts", Conflicts: conflicts}}
				return out, MergeConflictError{
					Worktree:  worktree,
					Conflicts: conflicts,
					Reason:    "Agent not available; resolve conflicts manually.",
				}
			}
			return output.MergeOutput{}, err
		}
		if _, err := agent.RunReview(context.Background(), provider, req); err != nil {
			return output.MergeOutput{}, err
		}
	}

	if err := gitDir(worktree, nil, "add", "-A"); err != nil {
		return output.MergeOutput{}, err
	}
	if unresolved := mergeConflictFiles(worktree); len(unresolved) > 0 {
		out := output.MergeOutput{Merge: output.MergeSummary{Status: "conflicts", Conflicts: unresolved}}
		return out, MergeConflictError{Worktree: worktree, Conflicts: unresolved}
	}
	treeSHA, err := gitOutputDir(worktree, "write-tree")
	if err != nil {
		return output.MergeOutput{}, err
	}
	mergedDraftSHA, err := gitutil.CreateDraftCommitFromTree(treeSHA, mergeBase, changeID)
	if err != nil {
		return output.MergeOutput{}, err
	}

	suggestion, err := metadata.CreateSuggestion(metadata.SuggestionCreate{
		ChangeID:           changeID,
		BaseCommitSHA:      mergeBase,
		SuggestedCommitSHA: mergedDraftSHA,
		CreatedBy:          "agent",
		Reason:             "merge_conflict",
		Description:        "conflict resolution",
	})
	if err != nil {
		return output.MergeOutput{}, err
	}

	out := output.MergeOutput{
		Merge: output.MergeSummary{
			Status:       "resolved",
			SuggestionID: suggestion.SuggestionID,
			Conflicts:    conflicts,
		},
		NextActions: []output.NextAction{
			{Action: "apply", Command: fmt.Sprintf("jul apply %s --json", suggestion.SuggestionID)},
			{Action: "reject", Command: fmt.Sprintf("jul reject %s --json", suggestion.SuggestionID)},
			{Action: "show", Command: fmt.Sprintf("jul show %s --json", suggestion.SuggestionID)},
		},
	}

	apply := autoApply
	if !autoApply {
		ok, err := promptYesNo("Accept merge resolution? [y/n] ")
		if err != nil {
			return out, err
		}
		apply = ok
	}

	if apply {
		if err := acceptMergeResolution(repoRoot, workspace, workspaceRef, syncRef, mergedDraftSHA, baseTarget, theirsSHA, remoteName); err != nil {
			return out, err
		}
		_, _ = metadata.UpdateSuggestionStatus(suggestion.SuggestionID, "applied", "merge accepted")
		out.Merge.Applied = true
	}

	return out, nil
}

func draftParentMismatch(draftSHA, mergeBase string) bool {
	msg, err := gitutil.CommitMessage(draftSHA)
	if err != nil {
		return false
	}
	if !isDraftMessage(msg) {
		return false
	}
	parent, err := gitutil.ParentOf(draftSHA)
	if err != nil || strings.TrimSpace(parent) == "" {
		return false
	}
	return strings.TrimSpace(parent) != strings.TrimSpace(mergeBase)
}

func changeIDFromCommit(sha string) string {
	msg, err := gitutil.CommitMessage(sha)
	if err == nil {
		if changeID := gitutil.ExtractChangeID(msg); changeID != "" {
			return changeID
		}
	}
	if strings.TrimSpace(sha) != "" {
		return gitutil.FallbackChangeID(sha)
	}
	if generated, err := gitutil.NewChangeID(); err == nil {
		return generated
	}
	return ""
}

func mergeConflictFiles(worktree string) []string {
	out, err := gitOutputDir(worktree, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	conflicts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		conflicts = append(conflicts, line)
	}
	return conflicts
}

type MergeConflictError struct {
	Worktree  string
	Conflicts []string
	Reason    string
}

func (e MergeConflictError) Error() string {
	if strings.TrimSpace(e.Reason) != "" {
		return e.Reason
	}
	return "conflicts detected"
}

func mergeConflictDetails(worktree string, files []string) []agent.ReviewFile {
	details := make([]agent.ReviewFile, 0, len(files))
	for _, path := range files {
		if strings.TrimSpace(path) == "" {
			continue
		}
		fullPath := filepath.Join(worktree, path)
		content, _ := os.ReadFile(fullPath)
		if len(content) > 64*1024 {
			content = content[:64*1024]
		}
		details = append(details, agent.ReviewFile{Path: path, Content: string(content)})
	}
	return details
}

func acceptMergeResolution(repoRoot, workspace, workspaceRef, syncRef, mergedSHA, baseSHA, oldWorkspace, remoteName string) error {
	if err := gitutil.UpdateRef(syncRef, mergedSHA); err != nil {
		return err
	}
	if strings.TrimSpace(baseSHA) != "" {
		if err := gitutil.UpdateRef(workspaceRef, baseSHA); err != nil {
			return err
		}
		if err := writeWorkspaceLease(repoRoot, workspace, baseSHA); err != nil {
			return err
		}
	}
	if err := updateWorktreeLocal(repoRoot, mergedSHA); err != nil {
		return err
	}
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	if err := pushRef(remoteName, mergedSHA, syncRef, true); err != nil {
		return err
	}
	if strings.TrimSpace(baseSHA) != "" {
		if err := pushWorkspace(remoteName, baseSHA, workspaceRef, oldWorkspace); err != nil {
			return err
		}
	}
	return nil
}

func gitOutputDirAllowErr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func fetchRef(remoteName, ref string) error {
	if strings.TrimSpace(remoteName) == "" || strings.TrimSpace(ref) == "" {
		return nil
	}
	_, err := gitutil.Git("fetch", remoteName, "+"+ref+":"+ref)
	return err
}

func pushRef(remoteName, sha, ref string, force bool) error {
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	spec := fmt.Sprintf("%s:%s", sha, ref)
	args := []string{"push"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, remoteName, spec)
	_, err := gitutil.Git(args...)
	return err
}

func pushWorkspace(remoteName, sha, ref, old string) error {
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	spec := fmt.Sprintf("%s:%s", sha, ref)
	args := []string{"push"}
	if strings.TrimSpace(old) != "" {
		args = append(args, "--force-with-lease="+ref+":"+old)
	}
	args = append(args, remoteName, spec)
	_, err := gitutil.Git(args...)
	return err
}

func promptYesNo(prompt string) (bool, error) {
	fmt.Fprint(os.Stdout, prompt)
	var response string
	if _, err := fmt.Fscanln(os.Stdin, &response); err != nil {
		return false, err
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}
