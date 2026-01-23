package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdoptCheckpointOnGitCommit(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	julPath := buildCLI(t)
	home := filepath.Join(t.TempDir(), "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
		"JUL_HOOK_CMD":  julPath,
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, env, julPath, "hooks", "install")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	if err := os.MkdirAll(filepath.Join(repo, ".jul"), 0o755); err != nil {
		t.Fatalf("failed to create .jul dir: %v", err)
	}
	config := "[checkpoint]\nadopt_on_commit = true\nadopt_run_ci = false\nadopt_run_review = false\n"
	if err := os.WriteFile(filepath.Join(repo, ".jul", "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, env, "git", "add", "README.md")
	runCmd(t, repo, env, "git", "commit", "-m", "feat: commit")

	head := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	refsOut := runCmd(t, repo, nil, "git", "show-ref")
	foundKeep := false
	for _, line := range strings.Split(strings.TrimSpace(refsOut), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[0] == head && strings.HasPrefix(fields[1], "refs/jul/keep/") {
			foundKeep = true
			break
		}
	}
	if !foundKeep {
		t.Fatalf("expected keep ref pointing at head")
	}

	workspaceRef := "refs/jul/workspaces/tester/@"
	draft := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", workspaceRef))
	parentsLine := runCmd(t, repo, nil, "git", "rev-list", "--parents", "-n", "1", draft)
	parts := strings.Fields(parentsLine)
	if len(parts) < 2 {
		t.Fatalf("expected draft to have parent, got %q", parentsLine)
	}
	if parts[1] != head {
		t.Fatalf("expected draft parent to be head, got %s", parts[1])
	}
}
