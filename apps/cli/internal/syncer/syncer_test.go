package syncer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestKeepRefPathIncludesUser(t *testing.T) {
	got := keepRefPath("george", "@", "Iabc", "def")
	want := "refs/jul/keep/george/@/Iabc/def"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCheckpointErrorsOnKeepRefPushFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(tmp, "remote.git")
	if err := run(tmp, "git", "init", "--bare", remoteDir); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(remoteDir, "hooks", "update")
	hook := "#!/bin/sh\nrefname=\"$1\"\ncase \"$refname\" in\n  refs/jul/keep/*) echo \"deny keep\" >&2; exit 1 ;;\nesac\nexit 0\n"
	if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := run(repoDir, "git", "remote", "add", "origin", remoteDir); err != nil {
		t.Fatal(err)
	}
	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(repoDir, "git", "push", "origin", "HEAD:"+workspaceRef); err != nil {
		t.Fatal(err)
	}
	workspaceSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	leasePath := filepath.Join(repoDir, ".jul", "workspaces", "@", "lease")
	if err := os.MkdirAll(filepath.Dir(leasePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leasePath, []byte(workspaceSHA+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if _, err := Checkpoint("feat: test"); err == nil {
		t.Fatalf("expected keep-ref push error, got nil")
	} else if !strings.Contains(err.Error(), "deny keep") && !strings.Contains(err.Error(), "refs/jul/keep") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncAutoMergeNoConflicts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	baseSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(tmp, "remote.git")
	if err := run(tmp, "git", "init", "--bare", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(remoteDir, "git", "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "remote", "add", "origin", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "push", "origin", "HEAD:refs/heads/main"); err != nil {
		t.Fatal(err)
	}

	// Create "theirs" draft commit from base.
	if err := os.WriteFile(filepath.Join(repoDir, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "remote.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "[draft] WIP\n\nChange-Id: Iremote"); err != nil {
		t.Fatal(err)
	}
	theirsSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(repoDir, "git", "push", "origin", theirsSHA+":"+workspaceRef); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "fetch", "origin", "+"+workspaceRef+":"+workspaceRef); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "reset", "--hard", baseSHA); err != nil {
		t.Fatal(err)
	}

	// Create local change (ours) from base.
	if err := os.WriteFile(filepath.Join(repoDir, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	leasePath := filepath.Join(repoDir, ".jul", "workspaces", "@", "lease")
	if err := os.MkdirAll(filepath.Dir(leasePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leasePath, []byte(baseSHA+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	res, err := Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !res.AutoMerged {
		t.Fatalf("expected auto-merge, got %+v", res)
	}
	if res.Diverged {
		t.Fatalf("expected no divergence after auto-merge")
	}
	mergedContent, err := gitOut(repoDir, "git", "show", res.WorkspaceRef+":remote.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(mergedContent) != "remote" {
		t.Fatalf("expected remote change in merge, got %q", mergedContent)
	}
	localMerged, err := gitOut(repoDir, "git", "show", res.WorkspaceRef+":local.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(localMerged) != "local" {
		t.Fatalf("expected local change in merge, got %q", localMerged)
	}
	localContent, err := os.ReadFile(filepath.Join(repoDir, "local.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(localContent)) != "local" {
		t.Fatalf("expected local change in working tree, got %q", string(localContent))
	}
}

func TestCheckpointKeepsChangeIDAcrossDrafts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	res, err := Checkpoint("feat: test")
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	if res.CheckpointSHA == "" || res.DraftSHA == "" {
		t.Fatalf("expected checkpoint and draft shas, got %+v", res)
	}
	checkpointMsg, err := gitOut(repoDir, "git", "log", "-1", "--format=%B", res.CheckpointSHA)
	if err != nil {
		t.Fatalf("failed to read checkpoint message: %v", err)
	}
	changeID := gitutil.ExtractChangeID(checkpointMsg)
	if changeID == "" {
		t.Fatalf("expected Change-Id in checkpoint message, got %q", checkpointMsg)
	}
	draftMsg, err := gitOut(repoDir, "git", "log", "-1", "--format=%B", res.DraftSHA)
	if err != nil {
		t.Fatalf("failed to read draft message: %v", err)
	}
	draftChangeID := gitutil.ExtractChangeID(draftMsg)
	if draftChangeID != changeID {
		t.Fatalf("expected new draft to keep Change-Id %s, got %s", changeID, draftChangeID)
	}
}

func TestSyncAssignsChangeIDToDraft(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	res, err := Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if res.DraftSHA == "" {
		t.Fatalf("expected draft sha in sync result")
	}
	draftMsg, err := gitOut(repoDir, "git", "log", "-1", "--format=%B", res.DraftSHA)
	if err != nil {
		t.Fatalf("failed to read draft message: %v", err)
	}
	if changeID := gitutil.ExtractChangeID(draftMsg); changeID == "" {
		t.Fatalf("expected Change-Id in draft message, got %q", draftMsg)
	}
}

func TestCheckpointAndAdoptErrorOnBaseDivergence(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	baseSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(tmp, "remote.git")
	if err := run(tmp, "git", "init", "--bare", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(remoteDir, "git", "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "remote", "add", "origin", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "push", "origin", "HEAD:refs/heads/main"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "base2.txt"), []byte("base2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base2.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "checkpoint2"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "remote.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "[draft] WIP\n\nChange-Id: Iremote"); err != nil {
		t.Fatal(err)
	}
	remoteDraftSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(repoDir, "git", "push", "origin", remoteDraftSHA+":"+workspaceRef); err != nil {
		t.Fatal(err)
	}

	if err := run(repoDir, "git", "reset", "--hard", strings.TrimSpace(baseSHA)); err != nil {
		t.Fatal(err)
	}
	leasePath := filepath.Join(repoDir, ".jul", "workspaces", "@", "lease")
	if err := os.MkdirAll(filepath.Dir(leasePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leasePath, []byte(strings.TrimSpace(baseSHA)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if _, err := Checkpoint("feat: test"); err == nil {
		t.Fatalf("expected divergence error on checkpoint")
	} else if !strings.Contains(err.Error(), "diverg") {
		t.Fatalf("expected divergence error, got %v", err)
	}
	if _, err := AdoptCheckpoint(); err == nil {
		t.Fatalf("expected divergence error on adopt")
	} else if !strings.Contains(err.Error(), "diverg") {
		t.Fatalf("expected divergence error, got %v", err)
	}

	refs, err := gitOut(repoDir, "git", "show-ref")
	if err != nil {
		t.Fatalf("failed to list refs: %v", err)
	}
	for _, line := range strings.Split(refs, "\n") {
		if strings.Contains(line, "refs/jul/keep/") {
			t.Fatalf("expected no keep refs, found %s", line)
		}
	}
}

func TestAdoptCheckpointErrorsOnMissingLease(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(tmp, "remote.git")
	if err := run(tmp, "git", "init", "--bare", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "remote", "add", "origin", remoteDir); err != nil {
		t.Fatal(err)
	}
	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(repoDir, "git", "push", "origin", "HEAD:"+workspaceRef); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if _, err := AdoptCheckpoint(); err == nil {
		t.Fatalf("expected divergence error on adopt")
	} else if !strings.Contains(err.Error(), "lease") {
		t.Fatalf("expected lease error, got %v", err)
	}

	refs, err := gitOut(repoDir, "git", "show-ref")
	if err != nil {
		t.Fatalf("failed to list refs: %v", err)
	}
	for _, line := range strings.Split(refs, "\n") {
		if strings.Contains(line, "refs/jul/keep/") {
			t.Fatalf("expected no keep refs, found %s", line)
		}
	}
}

func TestSyncDetectsBaseDivergence(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	baseSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(tmp, "remote.git")
	if err := run(tmp, "git", "init", "--bare", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "remote", "add", "origin", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "push", "origin", "HEAD:refs/heads/main"); err != nil {
		t.Fatal(err)
	}

	// Create a newer checkpoint base (base2) and a remote draft on top of it.
	if err := os.WriteFile(filepath.Join(repoDir, "base2.txt"), []byte("base2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base2.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "checkpoint2"); err != nil {
		t.Fatal(err)
	}
	base2SHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "remote.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "remote.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "[draft] WIP\n\nChange-Id: Iremote"); err != nil {
		t.Fatal(err)
	}
	remoteDraftSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(repoDir, "git", "push", "origin", remoteDraftSHA+":"+workspaceRef); err != nil {
		t.Fatal(err)
	}

	// Reset local repo back to the older base and set the workspace lease.
	if err := run(repoDir, "git", "reset", "--hard", strings.TrimSpace(baseSHA)); err != nil {
		t.Fatal(err)
	}
	leasePath := filepath.Join(repoDir, ".jul", "workspaces", "@", "lease")
	if err := os.MkdirAll(filepath.Dir(leasePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leasePath, []byte(strings.TrimSpace(baseSHA)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	res, err := Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !res.Diverged {
		t.Fatalf("expected divergence when bases differ (base %s vs %s)", strings.TrimSpace(baseSHA), strings.TrimSpace(base2SHA))
	}
	if res.AutoMerged {
		t.Fatalf("expected no auto-merge on base divergence")
	}
}

func TestSyncAutoMergeDeletesFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "gone.txt"), []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base.txt", "gone.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	baseSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(tmp, "remote.git")
	if err := run(tmp, "git", "init", "--bare", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(remoteDir, "git", "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "remote", "add", "origin", remoteDir); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "push", "origin", "HEAD:refs/heads/main"); err != nil {
		t.Fatal(err)
	}

	// Create "theirs" draft commit that deletes gone.txt in a separate clone.
	cloneDir := filepath.Join(tmp, "clone")
	if err := run(tmp, "git", "clone", remoteDir, cloneDir); err != nil {
		t.Fatal(err)
	}
	if err := run(cloneDir, "git", "config", "user.name", "Remote User"); err != nil {
		t.Fatal(err)
	}
	if err := run(cloneDir, "git", "config", "user.email", "remote@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := run(cloneDir, "git", "fetch", "origin", "refs/heads/main:refs/remotes/origin/main"); err != nil {
		t.Fatal(err)
	}
	if err := run(cloneDir, "git", "checkout", "main"); err != nil {
		t.Fatal(err)
	}
	if err := run(cloneDir, "git", "rm", "gone.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(cloneDir, "git", "commit", "-m", "[draft] WIP\n\nChange-Id: Iremote"); err != nil {
		t.Fatal(err)
	}
	theirsSHA, err := gitOut(cloneDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(cloneDir, "git", "push", "origin", theirsSHA+":"+workspaceRef); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "fetch", "origin", "+"+workspaceRef+":"+workspaceRef); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "reset", "--hard", baseSHA); err != nil {
		t.Fatal(err)
	}

	leasePath := filepath.Join(repoDir, ".jul", "workspaces", "@", "lease")
	if err := os.MkdirAll(filepath.Dir(leasePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leasePath, []byte(baseSHA+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	res, err := Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !res.AutoMerged {
		t.Fatalf("expected auto-merge, got %+v", res)
	}
	if res.Diverged {
		t.Fatalf("expected no divergence after auto-merge")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "gone.txt")); err == nil {
		t.Fatalf("expected gone.txt to be removed after auto-merge")
	}
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &execError{cmd: name + " " + strings.Join(args, " "), output: string(out)}
	}
	return nil
}

func gitOut(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", &execError{cmd: name + " " + strings.Join(args, " "), output: string(out)}
	}
	return strings.TrimSpace(string(out)), nil
}

type execError struct {
	cmd    string
	output string
}

func (e *execError) Error() string {
	return strings.TrimSpace(e.cmd + ": " + e.output)
}
