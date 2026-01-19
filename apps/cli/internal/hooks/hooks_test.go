package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndUninstallPostCommit(t *testing.T) {
	repo := t.TempDir()
	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	hookPath, err := InstallPostCommit(repo, "jul")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	installed, statusPath, err := StatusPostCommit(repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !installed {
		t.Fatalf("expected hook installed")
	}
	if hookPath != statusPath {
		t.Fatalf("expected path %s, got %s", hookPath, statusPath)
	}

	if err := UninstallPostCommit(repo); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	installed, _, err = StatusPostCommit(repo)
	if err != nil {
		t.Fatalf("status after uninstall failed: %v", err)
	}
	if installed {
		t.Fatalf("expected hook removed")
	}
}

func TestInstallDoesNotOverwriteForeignHook(t *testing.T) {
	repo := t.TempDir()
	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	hookPath := filepath.Join(hooksDir, postCommitHookName)
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("failed to write foreign hook: %v", err)
	}

	if _, err := InstallPostCommit(repo, "jul"); err == nil {
		t.Fatalf("expected error when foreign hook exists")
	}
}
