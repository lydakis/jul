package agent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWorktreeMergeState(t *testing.T) {
	repo := t.TempDir()
	if err := runGit(repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runGit(repo, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if err := runGit(repo, "config", "user.name", "Test"); err != nil {
		t.Fatalf("config name: %v", err)
	}

	filePath := filepath.Join(repo, "file.txt")
	if err := os.WriteFile(filePath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := runGit(repo, "add", "file.txt"); err != nil {
		t.Fatalf("add base: %v", err)
	}
	if err := runGit(repo, "commit", "-m", "base"); err != nil {
		t.Fatalf("commit base: %v", err)
	}
	baseSHA, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("base sha: %v", err)
	}

	if err := runGit(repo, "checkout", "-b", "other"); err != nil {
		t.Fatalf("checkout other: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("base\nother\n"), 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}
	if err := runGit(repo, "add", "file.txt"); err != nil {
		t.Fatalf("add other: %v", err)
	}
	if err := runGit(repo, "commit", "-m", "other"); err != nil {
		t.Fatalf("commit other: %v", err)
	}
	otherSHA, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("other sha: %v", err)
	}

	worktree, err := EnsureWorktree(repo, baseSHA, WorktreeOptions{})
	if err != nil {
		t.Fatalf("ensure worktree: %v", err)
	}

	if err := runGit(worktree, "merge", "--no-commit", "--no-ff", otherSHA); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !MergeInProgress(worktree) {
		t.Fatalf("expected merge in progress")
	}

	if _, err := EnsureWorktree(repo, baseSHA, WorktreeOptions{}); err == nil {
		t.Fatalf("expected merge-in-progress error")
	} else if !errors.Is(err, ErrMergeInProgress) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !MergeInProgress(worktree) {
		t.Fatalf("expected merge to remain in progress")
	}

	if err := runGit(worktree, "merge", "--abort"); err != nil {
		t.Fatalf("abort merge: %v", err)
	}
	if err := runGit(worktree, "merge", "--no-commit", "--no-ff", otherSHA); err != nil {
		t.Fatalf("merge again: %v", err)
	}
	if _, err := EnsureWorktree(repo, baseSHA, WorktreeOptions{AllowMergeInProgress: true}); err != nil {
		t.Fatalf("ensure worktree preserve: %v", err)
	}
	if !MergeInProgress(worktree) {
		t.Fatalf("expected merge to remain in progress")
	}
}

func TestEnsureWorktreePreservesDirtyStateWhenAllowed(t *testing.T) {
	repo := t.TempDir()
	if err := runGit(repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runGit(repo, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if err := runGit(repo, "config", "user.name", "Test"); err != nil {
		t.Fatalf("config name: %v", err)
	}

	filePath := filepath.Join(repo, "file.txt")
	if err := os.WriteFile(filePath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := runGit(repo, "add", "file.txt"); err != nil {
		t.Fatalf("add base: %v", err)
	}
	if err := runGit(repo, "commit", "-m", "base"); err != nil {
		t.Fatalf("commit base: %v", err)
	}
	baseSHA, err := gitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("base sha: %v", err)
	}

	worktree, err := EnsureWorktree(repo, baseSHA, WorktreeOptions{})
	if err != nil {
		t.Fatalf("ensure worktree: %v", err)
	}

	dirtyPath := filepath.Join(worktree, "file.txt")
	if err := os.WriteFile(dirtyPath, []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty: %v", err)
	}

	worktree2, err := EnsureWorktree(repo, baseSHA, WorktreeOptions{AllowMergeInProgress: true})
	if err != nil {
		t.Fatalf("ensure worktree preserve: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(worktree2, "file.txt"))
	if err != nil {
		t.Fatalf("read dirty: %v", err)
	}
	if strings.TrimSpace(string(data)) != "dirty" {
		t.Fatalf("expected dirty state to remain, got %q", strings.TrimSpace(string(data)))
	}
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
