package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmokeLocalOnlyFlow(t *testing.T) {
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

	// Sync draft locally
	writeFile(t, repo, "README.md", "hello\n")
	syncOut := runCmd(t, repo, env, julPath, "sync", "--json")
	var syncRes struct {
		DraftSHA     string `json:"DraftSHA"`
		WorkspaceRef string `json:"WorkspaceRef"`
		SyncRef      string `json:"SyncRef"`
	}
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&syncRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if syncRes.DraftSHA == "" || syncRes.WorkspaceRef == "" || syncRes.SyncRef == "" {
		t.Fatalf("expected sync refs, got %+v", syncRes)
	}
	runCmd(t, repo, nil, "git", "show-ref", syncRes.SyncRef)
	runCmd(t, repo, nil, "git", "show-ref", syncRes.WorkspaceRef)

	// Checkpoint locally (keep-ref)
	checkpointOut := runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--json")
	var checkpointRes struct {
		CheckpointSHA string `json:"CheckpointSHA"`
		KeepRef       string `json:"KeepRef"`
	}
	if err := json.NewDecoder(strings.NewReader(checkpointOut)).Decode(&checkpointRes); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if checkpointRes.KeepRef == "" || checkpointRes.CheckpointSHA == "" {
		t.Fatalf("expected keep ref and checkpoint sha")
	}
	runCmd(t, repo, nil, "git", "show-ref", checkpointRes.KeepRef)
}
