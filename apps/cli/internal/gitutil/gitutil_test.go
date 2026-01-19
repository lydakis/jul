package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitPathHooksWithCustomPath(t *testing.T) {
	repo := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	customHooks := filepath.Join(repo, "custom-hooks")
	cmd = exec.Command("git", "config", "core.hooksPath", customHooks)
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config core.hooksPath failed: %v", err)
	}

	hooksPath, err := GitPath(repo, "hooks")
	if err != nil {
		t.Fatalf("GitPath failed: %v", err)
	}
	if hooksPath != customHooks {
		t.Fatalf("expected %s, got %s", customHooks, hooksPath)
	}

	if err := os.MkdirAll(hooksPath, 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
}

func TestCurrentCommitUsesCommitTime(t *testing.T) {
	repo := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.name failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.email failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "test commit")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	cmd = exec.Command("git", "log", "-1", "--format=%cI")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	commitISO := strings.TrimSpace(string(out))
	commitTime, err := time.Parse(time.RFC3339, commitISO)
	if err != nil {
		t.Fatalf("parse commit time failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	info, err := CurrentCommit()
	if err != nil {
		t.Fatalf("CurrentCommit failed: %v", err)
	}
	if !info.Committed.Equal(commitTime) {
		t.Fatalf("expected commit time %s, got %s", commitTime.Format(time.RFC3339), info.Committed.Format(time.RFC3339))
	}
}
