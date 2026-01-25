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
	if !gitutil.RefExists(workspaceRef) {
		t.Fatalf("expected workspace ref %s", workspaceRef)
	}
	syncRef, err := syncRef(user, workspace)
	if err != nil {
		t.Fatalf("sync ref error: %v", err)
	}
	if !gitutil.RefExists(syncRef) {
		t.Fatalf("expected sync ref %s", syncRef)
	}

	sha, err := gitutil.ResolveRef(workspaceRef)
	if err != nil {
		t.Fatalf("resolve workspace ref failed: %v", err)
	}
	msg, _ := gitutil.CommitMessage(sha)
	if gitutil.ExtractChangeID(msg) == "" {
		t.Fatalf("expected Change-Id in draft message, got %q", msg)
	}

	leasePath := filepath.Join(repo, ".jul", "workspaces", workspace, "lease")
	data, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("expected workspace lease, got %v", err)
	}
	if strings.TrimSpace(string(data)) != strings.TrimSpace(sha) {
		t.Fatalf("expected lease %s, got %s", sha, strings.TrimSpace(string(data)))
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
	if parent, err := gitutil.ParentOf(sha); err == nil && strings.TrimSpace(parent) != "" {
		if strings.TrimSpace(cfg.BaseSHA) != strings.TrimSpace(parent) {
			t.Fatalf("expected base_sha %s, got %s", strings.TrimSpace(parent), strings.TrimSpace(cfg.BaseSHA))
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
	if !gitutil.RefExists(workspaceRef) {
		t.Fatalf("expected workspace ref %s", workspaceRef)
	}
}
