package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewWithOpenCode(t *testing.T) {
	if os.Getenv("JUL_REAL_AGENT") != "1" {
		t.Skip("set JUL_REAL_AGENT=1 to run real agent smoke test")
	}
	opencodeBin := strings.TrimSpace(os.Getenv("JUL_OPENCODE_BIN"))
	if opencodeBin == "" {
		t.Skip("set JUL_OPENCODE_BIN to a real opencode binary")
	}
	if _, err := os.Stat(opencodeBin); err != nil {
		t.Fatalf("opencode binary not found: %v", err)
	}

	repo := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	julPath := buildCLI(t)
	installBundledAgent(t, julPath, opencodeBin)

	env := map[string]string{
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, env, julPath, "sync")
	runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review")

	reviewOut := runCmd(t, repo, env, julPath, "review", "--json")
	var reviewRes struct {
		Review struct {
			Status string `json:"status"`
		} `json:"review"`
	}
	if err := json.NewDecoder(strings.NewReader(reviewOut)).Decode(&reviewRes); err != nil {
		t.Fatalf("failed to decode review output: %v", err)
	}
	if reviewRes.Review.Status == "" {
		t.Fatalf("expected review status")
	}
}

func installBundledAgent(t *testing.T, julPath, agentPath string) {
	t.Helper()
	libexecDir := filepath.Join(filepath.Dir(julPath), "libexec", "jul")
	if err := os.MkdirAll(libexecDir, 0o755); err != nil {
		t.Fatalf("failed to create libexec dir: %v", err)
	}
	dst := filepath.Join(libexecDir, "opencode")
	if err := copyFile(agentPath, dst); err != nil {
		t.Fatalf("failed to copy opencode: %v", err)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return err
	}
	return nil
}
