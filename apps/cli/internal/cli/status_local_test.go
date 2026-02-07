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
	draftRef, err := syncRef(user, workspace)
	if err != nil {
		t.Fatalf("sync ref error: %v", err)
	}
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

func TestParseWorkingTreePorcelainZClean(t *testing.T) {
	status := parseWorkingTreePorcelainZ("")
	if !status.Clean {
		t.Fatalf("expected clean status")
	}
	if len(status.Staged) != 0 || len(status.Unstaged) != 0 || len(status.Untracked) != 0 {
		t.Fatalf("expected empty entries, got %+v", status)
	}
}

func TestParseWorkingTreePorcelainZMixedEntries(t *testing.T) {
	raw := strings.Join([]string{
		"M  staged.go",
		" M unstaged.go",
		"MM both.go",
		"?? untracked file.txt",
		"D  deleted.go",
		"",
	}, "\x00")

	status := parseWorkingTreePorcelainZ(raw)
	if status.Clean {
		t.Fatalf("expected dirty status")
	}
	if len(status.Staged) != 3 {
		t.Fatalf("expected 3 staged entries, got %d (%+v)", len(status.Staged), status.Staged)
	}
	if len(status.Unstaged) != 2 {
		t.Fatalf("expected 2 unstaged entries, got %d (%+v)", len(status.Unstaged), status.Unstaged)
	}
	if len(status.Untracked) != 1 {
		t.Fatalf("expected 1 untracked entry, got %d (%+v)", len(status.Untracked), status.Untracked)
	}

	if status.Staged[0].Path != "staged.go" || status.Staged[0].Status != "M" {
		t.Fatalf("unexpected staged[0]: %+v", status.Staged[0])
	}
	if status.Staged[1].Path != "both.go" || status.Staged[1].Status != "M" {
		t.Fatalf("unexpected staged[1]: %+v", status.Staged[1])
	}
	if status.Staged[2].Path != "deleted.go" || status.Staged[2].Status != "D" {
		t.Fatalf("unexpected staged[2]: %+v", status.Staged[2])
	}

	if status.Unstaged[0].Path != "unstaged.go" || status.Unstaged[0].Status != "M" {
		t.Fatalf("unexpected unstaged[0]: %+v", status.Unstaged[0])
	}
	if status.Unstaged[1].Path != "both.go" || status.Unstaged[1].Status != "M" {
		t.Fatalf("unexpected unstaged[1]: %+v", status.Unstaged[1])
	}

	if status.Untracked[0].Path != "untracked file.txt" || status.Untracked[0].Status != "?" {
		t.Fatalf("unexpected untracked[0]: %+v", status.Untracked[0])
	}
}
