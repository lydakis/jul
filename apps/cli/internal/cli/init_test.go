package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	wsconfig "github.com/lydakis/jul/cli/internal/workspace"
)

func TestInitStartsDraftAndLease(t *testing.T) {
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if code := runInit([]string{"demo"}); code != 0 {
		t.Fatalf("init failed with %d", code)
	}

	deviceID, err := config.DeviceID()
	if err != nil || deviceID == "" {
		t.Fatalf("expected device id, got %v", err)
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	workspaceRef := workspaceRef(user, workspace)
	syncRef, err := syncRef(user, workspace)
	if err != nil {
		t.Fatalf("sync ref error: %v", err)
	}

	// If the repo has a base commit, workspace refs/lease should be set.
	if head, err := gitutil.Git("rev-parse", "HEAD"); err == nil && strings.TrimSpace(head) != "" {
		if !gitutil.RefExists(workspaceRef) {
			t.Fatalf("expected workspace ref %s", workspaceRef)
		}
		if !gitutil.RefExists(syncRef) {
			t.Fatalf("expected sync ref %s", syncRef)
		}
		sha, err := gitutil.ResolveRef(workspaceRef)
		if err != nil {
			t.Fatalf("resolve workspace ref failed: %v", err)
		}
		leasePath := filepath.Join(repo, ".jul", "workspaces", workspace, "lease")
		data, err := os.ReadFile(leasePath)
		if err != nil {
			t.Fatalf("expected workspace lease, got %v", err)
		}
		if strings.TrimSpace(string(data)) != strings.TrimSpace(sha) {
			t.Fatalf("expected lease %s, got %s", sha, strings.TrimSpace(string(data)))
		}
	}

	cfg, ok, err := wsconfig.ReadConfig(repo, workspace)
	if err != nil {
		t.Fatalf("read workspace config failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected workspace config to exist")
	}
	if strings.TrimSpace(cfg.BaseRef) == "" {
		t.Fatalf("expected base_ref to be set")
	}
	if head, err := gitutil.Git("rev-parse", "HEAD"); err == nil && strings.TrimSpace(head) != "" {
		if strings.TrimSpace(cfg.BaseSHA) != strings.TrimSpace(head) {
			t.Fatalf("expected base_sha %s, got %s", strings.TrimSpace(head), strings.TrimSpace(cfg.BaseSHA))
		}
	}
}

func TestInitWithMissingRemoteContinuesLocal(t *testing.T) {
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if code := runInit([]string{"--remote", "origin", "demo"}); code != 0 {
		t.Fatalf("init failed with %d", code)
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	workspaceRef := workspaceRef(user, workspace)
	if head, err := gitutil.Git("rev-parse", "HEAD"); err == nil && strings.TrimSpace(head) != "" {
		if !gitutil.RefExists(workspaceRef) {
			t.Fatalf("expected workspace ref %s", workspaceRef)
		}
	}
}

func TestEnsureJulIgnoredAddsExclude(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "info"), 0o755); err != nil {
		t.Fatalf("failed to create git info dir: %v", err)
	}

	if err := ensureJulIgnored(repo); err != nil {
		t.Fatalf("ensureJulIgnored failed: %v", err)
	}

	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("failed to read exclude file: %v", err)
	}
	if !strings.Contains(string(data), ".jul/") {
		t.Fatalf("expected .jul/ in exclude file, got %q", string(data))
	}
}
