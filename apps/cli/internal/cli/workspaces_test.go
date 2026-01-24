package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func TestWorkspaceNewCreatesDraftAndSavesCurrent(t *testing.T) {
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

	writeFilePath(t, repo, "a.txt", "from @\n")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if code := runWorkspaceNew([]string{"feature"}); code != 0 {
		t.Fatalf("ws new failed with %d", code)
	}

	if got := config.WorkspaceID(); got != "tester/feature" {
		t.Fatalf("expected workspace tester/feature, got %s", got)
	}

	states, err := localList()
	if err != nil {
		t.Fatalf("localList failed: %v", err)
	}
	found := false
	for _, state := range states {
		if state.Name == "@" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected local state for @ to be saved")
	}

	baseSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	workspaceRef := "refs/jul/workspaces/tester/feature"
	draftSHA, err := gitutil.ResolveRef(workspaceRef)
	if err != nil {
		t.Fatalf("failed to resolve workspace ref: %v", err)
	}
	parent, err := gitutil.ParentOf(draftSHA)
	if err != nil {
		t.Fatalf("failed to read draft parent: %v", err)
	}
	if strings.TrimSpace(parent) != baseSHA {
		t.Fatalf("expected draft parent %s, got %s", baseSHA, parent)
	}
	if got := strings.TrimSpace(readFileContents(t, repo, "a.txt")); got != "from @" {
		t.Fatalf("expected working tree to remain, got %q", got)
	}
}

func TestWorkspaceNewFailsWhenWorkspaceExists(t *testing.T) {
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

	if code := runWorkspaceNew([]string{"feature"}); code != 0 {
		t.Fatalf("ws new failed with %d", code)
	}

	ref := "refs/jul/workspaces/tester/feature"
	sha1, err := gitutil.ResolveRef(ref)
	if err != nil {
		t.Fatalf("failed to resolve workspace ref: %v", err)
	}

	if code := runWorkspaceNew([]string{"feature"}); code == 0 {
		t.Fatalf("expected ws new to fail for existing workspace")
	}

	sha2, err := gitutil.ResolveRef(ref)
	if err != nil {
		t.Fatalf("failed to resolve workspace ref: %v", err)
	}
	if strings.TrimSpace(sha1) != strings.TrimSpace(sha2) {
		t.Fatalf("expected workspace ref to remain unchanged")
	}
}

func TestWorkspaceSwitchRestoresLocalState(t *testing.T) {
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

	writeFilePath(t, repo, "a.txt", "from @\n")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if code := runWorkspaceNew([]string{"feature"}); code != 0 {
		t.Fatalf("ws new failed with %d", code)
	}

	writeFilePath(t, repo, "b.txt", "from feature\n")

	if code := runWorkspaceSwitch([]string{"@"}); code != 0 {
		t.Fatalf("ws switch failed with %d", code)
	}

	if got := config.WorkspaceID(); got != "tester/@" {
		t.Fatalf("expected workspace tester/@, got %s", got)
	}
	if got := strings.TrimSpace(readFileContents(t, repo, "a.txt")); got != "from @" {
		t.Fatalf("expected a.txt from @, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); err == nil {
		t.Fatalf("expected b.txt to be absent after switch")
	}
}

func TestWorkspaceSwitchKeepsConfigOnFailure(t *testing.T) {
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

	if code := runWorkspaceSwitch([]string{"missing"}); code == 0 {
		t.Fatalf("expected ws switch to fail for missing workspace")
	}
	if got := config.WorkspaceID(); got != "tester/@" {
		t.Fatalf("expected workspace to remain tester/@, got %s", got)
	}
}

func TestWorkspaceStackUsesCheckpointBase(t *testing.T) {
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

	writeFilePath(t, repo, "feature.txt", "feature\n")
	if _, err := syncer.Checkpoint("feat: base"); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	checkpoint, err := latestCheckpoint()
	if err != nil || checkpoint == nil {
		t.Fatalf("expected checkpoint")
	}

	if code := runWorkspaceStack([]string{"stacked"}); code != 0 {
		t.Fatalf("ws stack failed with %d", code)
	}

	workspaceRef := "refs/jul/workspaces/tester/stacked"
	draftSHA, err := gitutil.ResolveRef(workspaceRef)
	if err != nil {
		t.Fatalf("failed to resolve workspace ref: %v", err)
	}
	parent, err := gitutil.ParentOf(draftSHA)
	if err != nil {
		t.Fatalf("failed to read draft parent: %v", err)
	}
	if strings.TrimSpace(parent) != strings.TrimSpace(checkpoint.SHA) {
		t.Fatalf("expected stacked draft parent %s, got %s", checkpoint.SHA, parent)
	}
}

func TestWorkspaceStackFailsWhenWorkspaceExists(t *testing.T) {
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

	writeFilePath(t, repo, "feature.txt", "feature\n")
	if _, err := syncer.Checkpoint("feat: base"); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	if code := runWorkspaceNew([]string{"stacked"}); code != 0 {
		t.Fatalf("ws new failed with %d", code)
	}
	ref := "refs/jul/workspaces/tester/stacked"
	sha1, err := gitutil.ResolveRef(ref)
	if err != nil {
		t.Fatalf("failed to resolve workspace ref: %v", err)
	}

	if code := runWorkspaceSwitch([]string{"@"}); code != 0 {
		t.Fatalf("ws switch failed with %d", code)
	}
	if code := runWorkspaceStack([]string{"stacked"}); code == 0 {
		t.Fatalf("expected ws stack to fail for existing workspace")
	}

	sha2, err := gitutil.ResolveRef(ref)
	if err != nil {
		t.Fatalf("failed to resolve workspace ref: %v", err)
	}
	if strings.TrimSpace(sha1) != strings.TrimSpace(sha2) {
		t.Fatalf("expected stacked workspace ref to remain unchanged")
	}
}

func TestResetToDraftKeepsJulDir(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	writeFilePath(t, repo, "base.txt", "base\n")
	runGitCmd(t, repo, "add", "base.txt")
	runGitCmd(t, repo, "commit", "-m", "base")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	sha := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	julLocal := filepath.Join(repo, ".jul", "local")
	if err := os.MkdirAll(julLocal, 0o755); err != nil {
		t.Fatalf("failed to create .jul dir: %v", err)
	}
	writeFilePath(t, repo, ".jul/local/keep.txt", "keep\n")

	if err := resetToDraft(sha); err != nil {
		t.Fatalf("resetToDraft failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(julLocal, "keep.txt")); err != nil {
		t.Fatalf("expected .jul to remain after reset: %v", err)
	}
}

func readFileContents(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	return string(data)
}
