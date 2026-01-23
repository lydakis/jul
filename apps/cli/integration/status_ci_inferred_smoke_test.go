package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusShowsStaleCIWhenInferred(t *testing.T) {
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

	runCmd(t, repo, nil, "go", "mod", "init", "example.com/demo")
	writeFile(t, repo, "main.go", "package main\n\nfunc main() {}\n")

	runCmd(t, repo, env, julPath, "sync")
	runCmd(t, repo, env, julPath, "ci", "run", "--json")

	writeFile(t, repo, "CHANGELOG.md", "update\n")
	runCmd(t, repo, env, julPath, "sync")

	statusOut := runCmd(t, repo, env, julPath, "status", "--json")
	var status struct {
		DraftCI struct {
			Status         string `json:"status"`
			ResultsCurrent bool   `json:"results_current"`
		} `json:"draft_ci"`
	}
	if err := json.NewDecoder(strings.NewReader(statusOut)).Decode(&status); err != nil {
		t.Fatalf("failed to decode status output: %v", err)
	}
	if status.DraftCI.Status == "" {
		t.Fatalf("expected draft_ci status")
	}
	if status.DraftCI.ResultsCurrent {
		t.Fatalf("expected stale draft_ci results")
	}
}
