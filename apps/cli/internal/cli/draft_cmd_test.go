package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestDraftAdoptMerge(t *testing.T) {
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	runGitTestCmd(t, repo, "init")
	runGitTestCmd(t, repo, "config", "user.name", "Test User")
	runGitTestCmd(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base failed: %v", err)
	}
	runGitTestCmd(t, repo, "add", "base.txt")
	runGitTestCmd(t, repo, "commit", "-m", "base")

	baseSHA, err := gitWithDirTest(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatalf("mkdir remote failed: %v", err)
	}
	runGitTestCmd(t, remoteDir, "init", "--bare")
	runGitTestCmd(t, repo, "remote", "add", "origin", remoteDir)
	runGitTestCmd(t, repo, "push", "origin", "HEAD:refs/heads/main")

	if err := config.SetRepoConfigValue("remote", "name", "origin"); err != nil {
		t.Fatalf("set remote config failed: %v", err)
	}
	if err := config.SetRepoConfigValue("remote", "draft_sync", "enabled"); err != nil {
		t.Fatalf("set draft sync failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	changeID, err := gitutil.NewChangeID()
	if err != nil {
		t.Fatalf("new Change-Id failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("write local failed: %v", err)
	}
	localDraft, err := gitutil.CreateDraftCommit(strings.TrimSpace(baseSHA), changeID)
	if err != nil {
		t.Fatalf("create local draft failed: %v", err)
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		t.Fatalf("device id failed: %v", err)
	}
	localSyncRef := "refs/jul/sync/" + user + "/" + deviceID + "/" + workspace
	if err := gitutil.UpdateRef(localSyncRef, localDraft); err != nil {
		t.Fatalf("update local sync ref failed: %v", err)
	}

	// reset working tree to base and create remote draft
	runGitTestCmd(t, repo, "reset", "--hard", strings.TrimSpace(baseSHA))
	runGitTestCmd(t, repo, "clean", "-fd")
	if err := os.WriteFile(filepath.Join(repo, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatalf("write remote failed: %v", err)
	}
	remoteDraft, err := gitutil.CreateDraftCommit(strings.TrimSpace(baseSHA), changeID)
	if err != nil {
		t.Fatalf("create remote draft failed: %v", err)
	}
	remoteDevice := "other-device"
	remoteRef := "refs/jul/sync/" + user + "/" + remoteDevice + "/" + workspace
	if err := gitutil.UpdateRef(remoteRef, remoteDraft); err != nil {
		t.Fatalf("update remote ref failed: %v", err)
	}
	if err := pushRef("origin", remoteDraft, remoteRef, true); err != nil {
		t.Fatalf("push remote ref failed: %v", err)
	}

	res, err := adoptDraft(draftAdoptOptions{Device: remoteDevice}, nil)
	if err != nil {
		t.Fatalf("adopt draft failed: %v", err)
	}
	if res.DraftSHA == "" {
		t.Fatalf("expected new draft sha")
	}
	files, err := diffNameOnly(strings.TrimSpace(baseSHA), res.DraftSHA)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	got := strings.Join(files, ",")
	if !strings.Contains(got, "local.txt") || !strings.Contains(got, "remote.txt") {
		t.Fatalf("expected merged draft to include both files, got %v", files)
	}
}

func TestDraftAdoptBaseMismatch(t *testing.T) {
	repo := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	runGitTestCmd(t, repo, "init")
	runGitTestCmd(t, repo, "config", "user.name", "Test User")
	runGitTestCmd(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base failed: %v", err)
	}
	runGitTestCmd(t, repo, "add", "base.txt")
	runGitTestCmd(t, repo, "commit", "-m", "base")

	baseSHA, err := gitWithDirTest(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatalf("mkdir remote failed: %v", err)
	}
	runGitTestCmd(t, remoteDir, "init", "--bare")
	runGitTestCmd(t, repo, "remote", "add", "origin", remoteDir)
	runGitTestCmd(t, repo, "push", "origin", "HEAD:refs/heads/main")

	if err := config.SetRepoConfigValue("remote", "name", "origin"); err != nil {
		t.Fatalf("set remote config failed: %v", err)
	}
	if err := config.SetRepoConfigValue("remote", "draft_sync", "enabled"); err != nil {
		t.Fatalf("set draft sync failed: %v", err)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	changeID, err := gitutil.NewChangeID()
	if err != nil {
		t.Fatalf("new Change-Id failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("write local failed: %v", err)
	}
	localDraft, err := gitutil.CreateDraftCommit(strings.TrimSpace(baseSHA), changeID)
	if err != nil {
		t.Fatalf("create local draft failed: %v", err)
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		t.Fatalf("device id failed: %v", err)
	}
	localSyncRef := "refs/jul/sync/" + user + "/" + deviceID + "/" + workspace
	if err := gitutil.UpdateRef(localSyncRef, localDraft); err != nil {
		t.Fatalf("update local sync ref failed: %v", err)
	}

	// create a new base commit so remote draft has different base
	if err := os.WriteFile(filepath.Join(repo, "base2.txt"), []byte("base2\n"), 0o644); err != nil {
		t.Fatalf("write base2 failed: %v", err)
	}
	runGitTestCmd(t, repo, "add", "base2.txt")
	runGitTestCmd(t, repo, "commit", "-m", "base2")
	newBase, err := gitWithDirTest(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatalf("write remote failed: %v", err)
	}
	remoteDraft, err := gitutil.CreateDraftCommit(strings.TrimSpace(newBase), changeID)
	if err != nil {
		t.Fatalf("create remote draft failed: %v", err)
	}
	remoteDevice := "other-device"
	remoteRef := "refs/jul/sync/" + user + "/" + remoteDevice + "/" + workspace
	if err := gitutil.UpdateRef(remoteRef, remoteDraft); err != nil {
		t.Fatalf("update remote ref failed: %v", err)
	}
	if err := pushRef("origin", remoteDraft, remoteRef, true); err != nil {
		t.Fatalf("push remote ref failed: %v", err)
	}

	if _, err := adoptDraft(draftAdoptOptions{Device: remoteDevice}, nil); err == nil {
		t.Fatalf("expected base mismatch error")
	}
}

func runGitTestCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
}

func gitWithDirTest(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
