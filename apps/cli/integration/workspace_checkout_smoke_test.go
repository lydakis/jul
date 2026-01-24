package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceCheckoutDoesNotMoveBranch(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "init")

	julPath := buildCLI(t)
	home := filepath.Join(t.TempDir(), "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, env, julPath, "sync", "--json")

	headBefore := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, env, julPath, "ws", "checkout", "@")
	headAfter := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))

	if headAfter != headBefore {
		t.Fatalf("expected HEAD to remain %s, got %s", headBefore, headAfter)
	}
}
