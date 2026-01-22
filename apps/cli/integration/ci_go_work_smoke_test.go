package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIGoWorkDefaultCommands(t *testing.T) {
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

	moduleDir := filepath.Join(repo, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	runCmd(t, moduleDir, nil, "go", "mod", "init", "example.com/module")
	writeFile(t, moduleDir, "main.go", "package main\n\nfunc main() {}\n")

	runCmd(t, repo, nil, "go", "work", "init", "./module")

	runCmd(t, repo, env, julPath, "sync")

	ciOut := runCmd(t, repo, env, julPath, "ci", "--json")
	var ciRes struct {
		CI struct {
			Status  string `json:"status"`
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
		} `json:"ci"`
	}
	if err := json.NewDecoder(strings.NewReader(ciOut)).Decode(&ciRes); err != nil {
		t.Fatalf("failed to decode ci output: %v", err)
	}
	if ciRes.CI.Status == "" {
		t.Fatalf("expected ci status")
	}
	if len(ciRes.CI.Results) == 0 {
		t.Fatalf("expected ci results")
	}
}
