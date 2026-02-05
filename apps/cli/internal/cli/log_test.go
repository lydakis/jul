package cli

import (
	"os"
	"os/exec"
	"path/filepath"
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
