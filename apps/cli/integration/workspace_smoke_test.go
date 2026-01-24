package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceFlows(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	julPath := buildCLI(t)
	home := filepath.Join(t.TempDir(), "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, env, julPath, "sync")
	runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: base", "--no-ci", "--no-review")

	runCmd(t, repo, env, julPath, "ws", "new", "feature")
	runCmd(t, repo, nil, "git", "show-ref", "refs/jul/workspaces/tester/feature")

	runCmd(t, repo, env, julPath, "ws", "stack", "stacked")
	runCmd(t, repo, nil, "git", "show-ref", "refs/jul/workspaces/tester/stacked")

	out := runCmd(t, repo, env, julPath, "ws", "switch", "@")
	if !strings.Contains(out, "Switched to workspace") {
		t.Fatalf("expected switch output, got %s", out)
	}
}
