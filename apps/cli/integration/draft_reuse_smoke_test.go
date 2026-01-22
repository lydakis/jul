package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDraftReuseWhenNoChanges(t *testing.T) {
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
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "init")

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

	syncOut2 := runCmd(t, repo, env, julPath, "sync", "--json")
	var syncRes2 struct {
		DraftSHA string `json:"DraftSHA"`
	}
	if err := json.NewDecoder(strings.NewReader(syncOut2)).Decode(&syncRes2); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if syncRes2.DraftSHA != syncRes.DraftSHA {
		t.Fatalf("expected same draft sha on clean sync")
	}

	statusOut := runCmd(t, repo, env, julPath, "status", "--json")
	var status struct {
		Draft struct {
			FilesChanged int `json:"files_changed"`
		} `json:"draft"`
	}
	if err := json.NewDecoder(strings.NewReader(statusOut)).Decode(&status); err != nil {
		t.Fatalf("failed to decode status output: %v", err)
	}
	if status.Draft.FilesChanged != 0 {
		t.Fatalf("expected clean draft, got %d files changed", status.Draft.FilesChanged)
	}
}
