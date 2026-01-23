package cli

import (
	"context"
	"encoding/json"
	"flag"
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
			fs := flag.NewFlagSet("merge", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			autoApply := fs.Bool("apply", false, "Apply merge resolution without prompting")
			_ = fs.Parse(args)

			res, err := runMerge(*autoApply)
			if err != nil {
				fmt.Fprintf(os.Stderr, "merge failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
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

	worktree, err := agent.EnsureWorktree(repoRoot, oursSHA)
	if err != nil {
		return output.MergeOutput{}, err
	}
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

	changeID := changeIDFromCommit(mergeBase)
	if len(conflicts) > 0 {
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
		return output.MergeOutput{Merge: output.MergeSummary{Status: "conflicts", Conflicts: unresolved}}, fmt.Errorf("conflicts remain after merge")
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
		if err := acceptMergeResolution(repoRoot, workspace, workspaceRef, syncRef, mergedDraftSHA, theirsSHA, remoteName); err != nil {
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

func acceptMergeResolution(repoRoot, workspace, workspaceRef, syncRef, mergedSHA, oldWorkspace, remoteName string) error {
	if err := gitutil.UpdateRef(syncRef, mergedSHA); err != nil {
		return err
	}
	if err := gitutil.UpdateRef(workspaceRef, mergedSHA); err != nil {
		return err
	}
	if err := writeWorkspaceBase(repoRoot, workspace, mergedSHA); err != nil {
		return err
	}
	if err := updateWorktreeTo(repoRoot, mergedSHA); err != nil {
		return err
	}
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	if err := pushRef(remoteName, mergedSHA, syncRef, true); err != nil {
		return err
	}
	if err := pushWorkspace(remoteName, mergedSHA, workspaceRef, oldWorkspace); err != nil {
		return err
	}
	return nil
}

func updateWorktreeTo(repoRoot, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("ref required for worktree update")
	}
	if err := gitDir(repoRoot, nil, "checkout", "--force", ref, "--"); err != nil {
		return err
	}
	return gitDir(repoRoot, nil, "clean", "-fd")
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
