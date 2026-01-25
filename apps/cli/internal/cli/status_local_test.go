package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestStatusUsesDraftChangeIDWithoutCheckpoint(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	writeFilePath(t, repo, "README.md", "hello\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "init")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	t.Setenv("JUL_WORKSPACE", "tester/@")
	t.Setenv("HOME", filepath.Join(repo, "home"))

	if _, err := ensureWorkspaceReady(repo); err != nil {
		t.Fatalf("ensureWorkspaceReady failed: %v", err)
	}

	user, workspace := workspaceParts()
	draftRef := workspaceRef(user, workspace)
	draftSHA, err := gitutil.ResolveRef(draftRef)
	if err != nil || strings.TrimSpace(draftSHA) == "" {
		t.Fatalf("failed to resolve draft ref: %v", err)
	}
	msg, err := gitutil.CommitMessage(draftSHA)
	if err != nil {
		t.Fatalf("failed to read draft message: %v", err)
	}
	draftChangeID := gitutil.ExtractChangeID(msg)
	if draftChangeID == "" {
		t.Fatalf("expected Change-Id in draft message")
	}

	status, err := buildLocalStatus()
	if err != nil {
		t.Fatalf("buildLocalStatus failed: %v", err)
	}
	if status.ChangeID != draftChangeID {
		t.Fatalf("expected change id %s, got %s", draftChangeID, status.ChangeID)
	}
	if status.Draft == nil || status.Draft.ChangeID != draftChangeID {
		t.Fatalf("expected draft change id %s, got %+v", draftChangeID, status.Draft)
	}
}
