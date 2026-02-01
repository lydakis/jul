package syncer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestDraftPushAllowedDetectsSecret(t *testing.T) {
	repo := t.TempDir()
	runGitSyncTest(t, repo, "init")
	runGitSyncTest(t, repo, "config", "user.name", "Test User")
	runGitSyncTest(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base failed: %v", err)
	}
	runGitSyncTest(t, repo, "add", "base.txt")
	runGitSyncTest(t, repo, "commit", "-m", "base")

	baseSHA, err := gitWithDirSync(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "secret.txt"), []byte("api_key=secretvalue\n"), 0o644); err != nil {
		t.Fatalf("write secret failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	changeID, err := gitutil.NewChangeID()
	if err != nil {
		t.Fatalf("new Change-Id failed: %v", err)
	}
	draftSHA, err := gitutil.CreateDraftCommit(strings.TrimSpace(baseSHA), changeID)
	if err != nil {
		t.Fatalf("create draft failed: %v", err)
	}

	ok, reason, err := DraftPushAllowed(repo, baseSHA, draftSHA, false)
	if err != nil {
		t.Fatalf("draft push check failed: %v", err)
	}
	if ok || reason == "" {
		t.Fatalf("expected secret to be detected")
	}

	ok, _, err = DraftPushAllowed(repo, baseSHA, draftSHA, true)
	if err != nil {
		t.Fatalf("draft push check failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected allow-secrets to override")
	}
}

func TestDraftPushAllowedRespectsSyncIgnore(t *testing.T) {
	repo := t.TempDir()
	runGitSyncTest(t, repo, "init")
	runGitSyncTest(t, repo, "config", "user.name", "Test User")
	runGitSyncTest(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base failed: %v", err)
	}
	runGitSyncTest(t, repo, "add", "base.txt")
	runGitSyncTest(t, repo, "commit", "-m", "base")

	baseSHA, err := gitWithDirSync(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repo, ".jul"), 0o755); err != nil {
		t.Fatalf("mkdir .jul failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".jul", "syncignore"), []byte("secret.txt\n"), 0o644); err != nil {
		t.Fatalf("write syncignore failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "secret.txt"), []byte("api_key=secretvalue\n"), 0o644); err != nil {
		t.Fatalf("write secret failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	changeID, err := gitutil.NewChangeID()
	if err != nil {
		t.Fatalf("new Change-Id failed: %v", err)
	}
	draftSHA, err := gitutil.CreateDraftCommit(strings.TrimSpace(baseSHA), changeID)
	if err != nil {
		t.Fatalf("create draft failed: %v", err)
	}

	ok, reason, err := DraftPushAllowed(repo, baseSHA, draftSHA, false)
	if err != nil {
		t.Fatalf("draft push check failed: %v", err)
	}
	if !ok || reason != "" {
		t.Fatalf("expected syncignore to skip secret scan, got ok=%v reason=%s", ok, reason)
	}
}

func runGitSyncTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
}

func gitWithDirSync(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
