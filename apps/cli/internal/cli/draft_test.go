package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestCurrentDraftAndBaseUsesDraftParent(t *testing.T) {
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	if err := runGit(repo, "init"); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := runGit(repo, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("git config name failed: %v", err)
	}
	if err := runGit(repo, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("git config email failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err := runGit(repo, "add", "README.md"); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := runGit(repo, "commit", "-m", "base"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	baseSHA, err := gitutil.Git("rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	changeID, err := gitutil.NewChangeID()
	if err != nil {
		t.Fatalf("new Change-Id failed: %v", err)
	}
	draftSHA, err := gitutil.CreateDraftCommit(strings.TrimSpace(baseSHA), changeID)
	if err != nil {
		t.Fatalf("create draft failed: %v", err)
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		t.Fatalf("device id failed: %v", err)
	}
	syncRef := filepath.ToSlash(
		strings.TrimSpace("refs/jul/sync/" + user + "/" + deviceID + "/" + workspace),
	)
	if err := gitutil.UpdateRef(syncRef, draftSHA); err != nil {
		t.Fatalf("update sync ref failed: %v", err)
	}

	gotDraft, gotBase, err := currentDraftAndBase()
	if err != nil {
		t.Fatalf("currentDraftAndBase failed: %v", err)
	}
	if strings.TrimSpace(gotDraft) != strings.TrimSpace(draftSHA) {
		t.Fatalf("expected draft %s, got %s", draftSHA, gotDraft)
	}
	if strings.TrimSpace(gotBase) != strings.TrimSpace(baseSHA) {
		t.Fatalf("expected base %s, got %s", baseSHA, gotBase)
	}
}
