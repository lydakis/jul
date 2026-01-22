package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDraftCIRunsOnSyncBlocking(t *testing.T) {
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

	if err := os.MkdirAll(filepath.Join(repo, ".jul"), 0o755); err != nil {
		t.Fatalf("failed to create .jul dir: %v", err)
	}
	config := "[ci]\nrun_on_draft = true\ndraft_ci_blocking = true\n"
	if err := os.WriteFile(filepath.Join(repo, ".jul", "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	runCmd(t, repo, env, julPath, "ci", "config", "--set", "smoke=true")

	syncOut := runCmd(t, repo, env, julPath, "sync", "--json")
	var syncRes struct {
		DraftSHA string `json:"DraftSHA"`
	}
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&syncRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if syncRes.DraftSHA == "" {
		t.Fatalf("expected draft sha")
	}

	ciStatusOut := runCmd(t, repo, env, julPath, "ci", "status", "--json")
	var status struct {
		CI struct {
			Status          string `json:"status"`
			CompletedSHA    string `json:"completed_sha"`
			CurrentDraftSHA string `json:"current_draft_sha"`
			Results         []struct {
				Name string `json:"name"`
			} `json:"results"`
		} `json:"ci"`
	}
	if err := json.NewDecoder(strings.NewReader(ciStatusOut)).Decode(&status); err != nil {
		t.Fatalf("failed to decode ci status: %v", err)
	}
	if status.CI.CompletedSHA == "" {
		t.Fatalf("expected completed sha")
	}
	if status.CI.CurrentDraftSHA != syncRes.DraftSHA {
		t.Fatalf("expected ci current draft sha to match sync draft")
	}
	if status.CI.Status == "" {
		t.Fatalf("expected ci status")
	}
	if len(status.CI.Results) == 0 || status.CI.Results[0].Name != "true" {
		t.Fatalf("expected ci results from config, got %+v", status.CI.Results)
	}
}
