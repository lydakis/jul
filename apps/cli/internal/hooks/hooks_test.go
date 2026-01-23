package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAndUninstallPostCommit(t *testing.T) {
	repo := t.TempDir()
	if err := initGitRepo(repo); err != nil {
		t.Fatalf("git init failed: %v", err)
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
	if err := initGitRepo(repo); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	hooksDir, err := hooksPath(repo)
	if err != nil {
		t.Fatalf("hooks path failed: %v", err)
	}

	hookPath := filepath.Join(hooksDir, postCommitHookName)
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("failed to write foreign hook: %v", err)
	}

	if _, err := InstallPostCommit(repo, "jul"); err == nil {
		t.Fatalf("expected error when foreign hook exists")
	}
}

func TestInstallUsesCustomHooksPath(t *testing.T) {
	repo := t.TempDir()
	if err := initGitRepo(repo); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	customHooks := filepath.Join(repo, "custom-hooks")
	cmd := exec.Command("git", "config", "core.hooksPath", customHooks)
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config core.hooksPath failed: %v", err)
	}

	hookPath, err := InstallPostCommit(repo, "jul")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if !strings.HasPrefix(hookPath, customHooks) {
		t.Fatalf("expected hook path under %s, got %s", customHooks, hookPath)
	}
}

func initGitRepo(dir string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	return cmd.Run()
}

func hooksPath(repo string) (string, error) {
	cmd := exec.Command("git", "-C", repo, "rev-parse", "--git-path", "hooks")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(output))
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(repo, path), nil
}
