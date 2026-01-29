package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

func TestPromoteRecordsChangeMeta(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	changeID := "I2222222222222222222222222222222222222222"
	writeFilePath(t, repo, "README.md", "hello\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "test commit\n\nChange-Id: "+changeID)
	sha := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	keepRef := "refs/jul/keep/tester/@/" + changeID + "/" + sha
	runGitCmd(t, repo, "update-ref", keepRef, sha)

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("JUL_WORKSPACE", "tester/@")
	t.Setenv("HOME", filepath.Join(repo, "home"))
	if err := promoteLocal("main", sha, false); err != nil {
		t.Fatalf("promote failed: %v", err)
	}

	meta, ok, err := metadata.ReadChangeMeta(sha)
	if err != nil {
		t.Fatalf("ReadChangeMeta failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected change meta note to exist")
	}
	if meta.ChangeID != changeID {
		t.Fatalf("expected change id %s, got %s", changeID, meta.ChangeID)
	}
	if meta.AnchorSHA != sha {
		t.Fatalf("expected anchor sha %s, got %s", sha, meta.AnchorSHA)
	}
	if len(meta.PromoteEvents) != 1 {
		t.Fatalf("expected 1 promote event, got %d", len(meta.PromoteEvents))
	}
	if meta.PromoteEvents[0].Target != "main" {
		t.Fatalf("expected target main, got %s", meta.PromoteEvents[0].Target)
	}
	if len(meta.PromoteEvents[0].Published) != 1 || meta.PromoteEvents[0].Published[0] != sha {
		t.Fatalf("expected published [%s], got %+v", sha, meta.PromoteEvents[0].Published)
	}
}

func TestPromoteStartsNewDraftWithNewChangeID(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	changeID := "I3333333333333333333333333333333333333333"
	writeFilePath(t, repo, "README.md", "hello\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "test commit\n\nChange-Id: "+changeID)
	sha := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	keepRef := "refs/jul/keep/tester/@/" + changeID + "/" + sha
	runGitCmd(t, repo, "update-ref", keepRef, sha)

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("JUL_WORKSPACE", "tester/@")
	t.Setenv("HOME", filepath.Join(repo, "home"))
	deviceID, err := config.DeviceID()
	if err != nil {
		t.Fatalf("failed to get device id: %v", err)
	}

	if err := promoteLocal("main", sha, false); err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	headSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	if headSHA != sha {
		t.Fatalf("expected HEAD to remain %s after promote, got %s", sha, headSHA)
	}

	user, workspace := workspaceParts()
	workspaceRef := workspaceRef(user, workspace)
	baseSHA, err := gitutil.ResolveRef(workspaceRef)
	if err != nil {
		t.Fatalf("failed to resolve workspace ref: %v", err)
	}
	if strings.TrimSpace(baseSHA) != sha {
		t.Fatalf("expected workspace ref to remain on promoted sha %s, got %s", sha, baseSHA)
	}

	syncRef := "refs/jul/sync/" + user + "/" + deviceID + "/" + workspace
	draftSHA, err := gitutil.ResolveRef(syncRef)
	if err != nil {
		t.Fatalf("failed to resolve sync ref: %v", err)
	}
	if strings.TrimSpace(draftSHA) == "" {
		t.Fatalf("expected new draft sha")
	}

	draftMsg, err := gitutil.CommitMessage(draftSHA)
	if err != nil {
		t.Fatalf("failed to read draft message: %v", err)
	}
	draftChangeID := gitutil.ExtractChangeID(draftMsg)
	if draftChangeID == "" {
		t.Fatalf("expected Change-Id in new draft message")
	}
	if draftChangeID == changeID {
		t.Fatalf("expected new Change-Id after promote, got %s", draftChangeID)
	}

	if strings.TrimSpace(draftSHA) == sha {
		t.Fatalf("expected new draft sha to differ from promoted sha")
	}
}
