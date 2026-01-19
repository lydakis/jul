package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
