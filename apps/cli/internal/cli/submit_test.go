package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func TestSubmitRequiresCheckpoint(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	writeFilePath(t, repo, "base.txt", "base\n")
	runGitCmd(t, repo, "add", "base.txt")
	runGitCmd(t, repo, "commit", "-m", "base")

	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	runGitCmd(t, repo, "config", "jul.workspace", "tester/@")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if code := runSubmit([]string{}); code == 0 {
		t.Fatalf("expected submit to fail without checkpoint")
	}
}

func TestSubmitWritesChangeRequestState(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	writeFilePath(t, repo, "base.txt", "base\n")
	runGitCmd(t, repo, "add", "base.txt")
	runGitCmd(t, repo, "commit", "-m", "base")

	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	runGitCmd(t, repo, "config", "jul.workspace", "tester/@")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	writeFilePath(t, repo, "feature.txt", "first\n")
	first, err := syncer.Checkpoint("feat: first")
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	writeFilePath(t, repo, "feature.txt", "second\n")
	second, err := syncer.Checkpoint("feat: second")
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	if code := runSubmit([]string{}); code != 0 {
		t.Fatalf("submit failed with %d", code)
	}

	state, ok, err := metadata.ReadChangeRequestState(first.CheckpointSHA)
	if err != nil {
		t.Fatalf("read cr state failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected cr state note")
	}
	if state.ChangeID != first.ChangeID {
		t.Fatalf("expected Change-Id %s, got %s", first.ChangeID, state.ChangeID)
	}
	if state.LatestCheckpoint != second.CheckpointSHA {
		t.Fatalf("expected latest checkpoint %s, got %s", second.CheckpointSHA, state.LatestCheckpoint)
	}
	if state.AnchorSHA != first.CheckpointSHA {
		t.Fatalf("expected anchor %s, got %s", first.CheckpointSHA, state.AnchorSHA)
	}
}

func TestSubmitRequiresCheckpointForCurrentChange(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	writeFilePath(t, repo, "base.txt", "base\n")
	runGitCmd(t, repo, "add", "base.txt")
	runGitCmd(t, repo, "commit", "-m", "base")

	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	runGitCmd(t, repo, "config", "jul.workspace", "tester/@")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	writeFilePath(t, repo, "feature.txt", "first\n")
	checkpoint, err := syncer.Checkpoint("feat: first")
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	if _, err := promoteLocal(promoteOptions{Branch: "main", TargetSHA: checkpoint.CheckpointSHA}); err != nil {
		t.Fatalf("promote failed: %v", err)
	}

	if code := runSubmit([]string{}); code == 0 {
		t.Fatalf("expected submit to fail without checkpoint for current change")
	}
}
