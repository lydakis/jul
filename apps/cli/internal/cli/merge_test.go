package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestRunMergeResetsStaleWorktreeOnRefMismatch(t *testing.T) {
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	runGitTestCmd(t, repo, "init")
	runGitTestCmd(t, repo, "config", "user.name", "Test User")
	runGitTestCmd(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base failed: %v", err)
	}
	runGitTestCmd(t, repo, "add", "conflict.txt")
	runGitTestCmd(t, repo, "commit", "-m", "base")

	baseSHA, err := gitWithDirTest(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	changeID, err := gitutil.NewChangeID()
	if err != nil {
		t.Fatalf("new Change-Id failed: %v", err)
	}
	baseSHA = strings.TrimSpace(baseSHA)

	createDraft := func(content string) string {
		runGitTestCmd(t, repo, "reset", "--hard", baseSHA)
		runGitTestCmd(t, repo, "clean", "-fd")
		if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte(content+"\n"), 0o644); err != nil {
			t.Fatalf("write draft content failed: %v", err)
		}
		sha, err := gitutil.CreateDraftCommit(baseSHA, changeID)
		if err != nil {
			t.Fatalf("create draft failed: %v", err)
		}
		return sha
	}

	ours1 := createDraft("ours-one")
	theirs1 := createDraft("theirs-one")

	deviceID, err := config.DeviceID()
	if err != nil {
		t.Fatalf("device id failed: %v", err)
	}
	user := "tester"
	workspace := "@"
	syncRef := "refs/jul/sync/" + user + "/" + deviceID + "/" + workspace
	workspaceRef := "refs/jul/workspaces/" + user + "/" + workspace

	if err := gitutil.UpdateRef(syncRef, ours1); err != nil {
		t.Fatalf("update sync ref failed: %v", err)
	}
	if err := gitutil.UpdateRef(workspaceRef, theirs1); err != nil {
		t.Fatalf("update workspace ref failed: %v", err)
	}

	var conflictErr MergeConflictError
	if _, err := runMerge(false, nil); err == nil || !errors.As(err, &conflictErr) {
		t.Fatalf("expected merge conflict, got %v", err)
	}

	worktree := filepath.Join(repo, ".jul", "agent-workspace", "worktree")
	if err := os.WriteFile(filepath.Join(worktree, "conflict.txt"), []byte("stale resolution\n"), 0o644); err != nil {
		t.Fatalf("write stale resolution failed: %v", err)
	}

	ours2 := createDraft("ours-two")
	theirs2 := createDraft("theirs-two")
	if err := gitutil.UpdateRef(syncRef, ours2); err != nil {
		t.Fatalf("update sync ref failed: %v", err)
	}
	if err := gitutil.UpdateRef(workspaceRef, theirs2); err != nil {
		t.Fatalf("update workspace ref failed: %v", err)
	}

	if _, err := runMerge(false, nil); err == nil || !errors.As(err, &conflictErr) {
		t.Fatalf("expected merge conflict after ref mismatch, got %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(worktree, "conflict.txt"))
	if err != nil {
		t.Fatalf("read conflict file failed: %v", err)
	}
	text := string(contents)
	if strings.Contains(text, "stale resolution") {
		t.Fatalf("expected stale resolution to be cleared, got %s", text)
	}
	if !strings.Contains(text, "ours-two") || !strings.Contains(text, "theirs-two") {
		t.Fatalf("expected refreshed conflict markers, got %s", text)
	}
}

func TestRunMergeUsesMergeHeadWhenRefsMatch(t *testing.T) {
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	runGitTestCmd(t, repo, "init")
	runGitTestCmd(t, repo, "config", "user.name", "Test User")
	runGitTestCmd(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base failed: %v", err)
	}
	runGitTestCmd(t, repo, "add", "conflict.txt")
	runGitTestCmd(t, repo, "commit", "-m", "base")

	baseSHA, err := gitWithDirTest(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	changeID, err := gitutil.NewChangeID()
	if err != nil {
		t.Fatalf("new Change-Id failed: %v", err)
	}
	baseSHA = strings.TrimSpace(baseSHA)

	createDraft := func(content string) string {
		runGitTestCmd(t, repo, "reset", "--hard", baseSHA)
		runGitTestCmd(t, repo, "clean", "-fd")
		if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte(content+"\n"), 0o644); err != nil {
			t.Fatalf("write draft content failed: %v", err)
		}
		sha, err := gitutil.CreateDraftCommit(baseSHA, changeID)
		if err != nil {
			t.Fatalf("create draft failed: %v", err)
		}
		return sha
	}

	ours := createDraft("ours")
	theirs := createDraft("theirs")

	deviceID, err := config.DeviceID()
	if err != nil {
		t.Fatalf("device id failed: %v", err)
	}
	user := "tester"
	workspace := "@"
	syncRef := "refs/jul/sync/" + user + "/" + deviceID + "/" + workspace
	workspaceRef := "refs/jul/workspaces/" + user + "/" + workspace

	if err := gitutil.UpdateRef(syncRef, ours); err != nil {
		t.Fatalf("update sync ref failed: %v", err)
	}
	if err := gitutil.UpdateRef(workspaceRef, ours); err != nil {
		t.Fatalf("update workspace ref failed: %v", err)
	}

	worktree, err := agent.EnsureWorktree(repo, ours, agent.WorktreeOptions{AllowMergeInProgress: true})
	if err != nil {
		t.Fatalf("ensure worktree failed: %v", err)
	}
	cmd := exec.Command("git", "merge", "--no-commit", "--no-ff", theirs)
	cmd.Dir = worktree
	output, mergeErr := cmd.CombinedOutput()
	if mergeErr != nil && len(mergeConflictFiles(worktree)) == 0 {
		t.Fatalf("merge failed without conflicts: %s", strings.TrimSpace(string(output)))
	}
	if len(mergeConflictFiles(worktree)) == 0 {
		t.Fatalf("expected conflicts to be present")
	}

	if err := os.WriteFile(filepath.Join(worktree, "conflict.txt"), []byte("manual resolution\n"), 0o644); err != nil {
		t.Fatalf("write manual resolution failed: %v", err)
	}
	addCmd := exec.Command("git", "add", "conflict.txt")
	addCmd.Dir = worktree
	if addOut, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s", strings.TrimSpace(string(addOut)))
	}

	gitPathCmd := exec.Command("git", "rev-parse", "--git-path", "MERGE_HEAD")
	gitPathCmd.Dir = worktree
	gitPathOut, err := gitPathCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse --git-path failed: %s", strings.TrimSpace(string(gitPathOut)))
	}
	if mergeHeadPath := strings.TrimSpace(string(gitPathOut)); mergeHeadPath != "" {
		_ = os.Remove(mergeHeadPath)
	}

	out, err := runMerge(true, nil)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if out.Merge.Status == "up_to_date" {
		t.Fatalf("expected merge to proceed when MERGE_HEAD exists")
	}
}
