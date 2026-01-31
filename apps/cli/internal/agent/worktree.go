package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type WorktreeOptions struct {
	AllowMergeInProgress bool
}

var ErrMergeInProgress = errors.New("merge in progress in agent worktree")

func EnsureWorktree(repoRoot, baseSHA string, opts WorktreeOptions) (string, error) {
	agentRoot := filepath.Join(repoRoot, ".jul", "agent-workspace")
	worktree := filepath.Join(agentRoot, "worktree")

	if err := os.MkdirAll(agentRoot, 0o755); err != nil {
		return "", err
	}

	if _, err := os.Stat(worktree); err == nil {
		if MergeInProgress(worktree) {
			if opts.AllowMergeInProgress {
				return worktree, nil
			}
			return "", ErrMergeInProgress
		}
		if err := resetWorktree(worktree, baseSHA); err == nil {
			return worktree, nil
		}
		_ = removeWorktree(repoRoot, worktree)
	}

	if err := addWorktree(repoRoot, worktree, baseSHA); err != nil {
		return "", err
	}
	if err := resetWorktree(worktree, baseSHA); err != nil {
		return "", err
	}
	return worktree, nil
}

func MergeInProgress(worktree string) bool {
	cmd := exec.Command("git", "-C", worktree, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func addWorktree(repoRoot, worktree, baseSHA string) error {
	return runGit(repoRoot, "worktree", "add", worktree, baseSHA)
}

func removeWorktree(repoRoot, worktree string) error {
	return runGit(repoRoot, "worktree", "remove", "--force", worktree)
}

func resetWorktree(worktree, baseSHA string) error {
	if err := runGit(worktree, "reset", "--hard", baseSHA); err != nil {
		return err
	}
	if err := runGit(worktree, "clean", "-fd"); err != nil {
		return err
	}
	return nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return nil
}
