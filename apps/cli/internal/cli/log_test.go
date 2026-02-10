package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestListCheckpoints(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	changeID := "I1111111111111111111111111111111111111111"
	writeFilePath(t, repo, "README.md", "hello\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "test commit\n\nChange-Id: "+changeID)
	sha := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	keepRef := "refs/jul/keep/tester/@/" + changeID + "/" + sha
	runGitCmd(t, repo, "update-ref", keepRef, sha)

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	t.Setenv("JUL_WORKSPACE", "tester/@")

	entries, err := listCheckpoints(0)
	if err != nil {
		t.Fatalf("listCheckpoints failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(entries))
	}
	if entries[0].SHA != sha {
		t.Fatalf("expected sha %s, got %s", sha, entries[0].SHA)
	}
}

func TestLatestCheckpointUsesBoundedLookup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git wrapper script is POSIX-specific")
	}

	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	writeFilePath(t, repo, "README.md", "hello\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "test commit\n\nChange-Id: I1111111111111111111111111111111111111111")
	sha := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	for i := 0; i < 64; i++ {
		changeID := fmt.Sprintf("I%040d", i+1)
		keepRef := "refs/jul/keep/tester/@/" + changeID + "/" + sha
		runGitCmd(t, repo, "update-ref", keepRef, sha)
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	t.Setenv("JUL_WORKSPACE", "tester/@")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("failed to resolve git: %v", err)
	}
	wrapperDir := t.TempDir()
	countPath := filepath.Join(wrapperDir, "git-calls.log")
	wrapperPath := filepath.Join(wrapperDir, "git")
	script := "#!/bin/sh\n" +
		"echo 1 >> \"$JUL_GIT_COUNT_FILE\"\n" +
		"exec \"" + realGit + "\" \"$@\"\n"
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write git wrapper: %v", err)
	}

	t.Setenv("JUL_GIT_COUNT_FILE", countPath)
	t.Setenv("PATH", wrapperDir+":"+os.Getenv("PATH"))

	checkpoint, err := latestCheckpoint()
	if err != nil {
		t.Fatalf("latestCheckpoint failed: %v", err)
	}
	if checkpoint == nil || strings.TrimSpace(checkpoint.SHA) != sha {
		t.Fatalf("expected latest checkpoint %s, got %#v", sha, checkpoint)
	}

	rawCount, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("failed to read git call counter: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(rawCount)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	if count > 20 {
		t.Fatalf("expected latestCheckpoint to use bounded git calls, got %d", count)
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s", strings.Join(args, " "), string(out))
	}
	return string(out)
}

func writeFilePath(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}
