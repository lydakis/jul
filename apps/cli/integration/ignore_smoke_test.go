package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncWithJulIgnored(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	julPath := buildCLI(t)
	env := map[string]string{
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, ".gitignore", ".jul/\n")
	writeFile(t, repo, "README.md", "hello\n")

	runCmd(t, repo, env, julPath, "sync")
}
