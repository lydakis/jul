package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSaveRestore(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	writeFilePath(t, repo, "a.txt", "one\n")
	runGitCmd(t, repo, "add", "a.txt")
	runGitCmd(t, repo, "commit", "-m", "base")

	// Stage "two", then modify to "three".
	writeFilePath(t, repo, "a.txt", "two\n")
	runGitCmd(t, repo, "add", "a.txt")
	writeFilePath(t, repo, "a.txt", "three\n")
	writeFilePath(t, repo, "b.txt", "untracked\n")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if _, err := localSave("snap1"); err != nil {
		t.Fatalf("localSave failed: %v", err)
	}

	// Change working tree to ensure restore really happens.
	runGitCmd(t, repo, "reset", "--hard", "HEAD")
	_ = os.Remove(filepath.Join(repo, "b.txt"))
	writeFilePath(t, repo, "a.txt", "changed\n")

	if err := localRestore("snap1"); err != nil {
		t.Fatalf("localRestore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt failed: %v", err)
	}
	if strings.TrimSpace(string(content)) != "three" {
		t.Fatalf("expected working tree to be 'three', got %q", strings.TrimSpace(string(content)))
	}
	indexContent := runGitCmd(t, repo, "show", ":a.txt")
	if strings.TrimSpace(indexContent) != "two" {
		t.Fatalf("expected index to be 'two', got %q", strings.TrimSpace(indexContent))
	}
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); err != nil {
		t.Fatalf("expected untracked file to be restored: %v", err)
	}
}

func TestLocalListDelete(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	writeFilePath(t, repo, "a.txt", "one\n")
	runGitCmd(t, repo, "add", "a.txt")
	runGitCmd(t, repo, "commit", "-m", "base")
	writeFilePath(t, repo, "a.txt", "two\n")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if _, err := localSave("snap1"); err != nil {
		t.Fatalf("localSave failed: %v", err)
	}

	states, err := localList()
	if err != nil {
		t.Fatalf("localList failed: %v", err)
	}
	if len(states) != 1 || states[0].Name != "snap1" {
		t.Fatalf("expected snap1 in list, got %+v", states)
	}

	if err := localDelete("snap1"); err != nil {
		t.Fatalf("localDelete failed: %v", err)
	}
	states, err = localList()
	if err != nil {
		t.Fatalf("localList failed: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected empty list after delete, got %+v", states)
	}
}
