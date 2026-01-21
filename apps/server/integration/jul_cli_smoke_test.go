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
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: base")
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
	checkpointOut := runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review", "--json")
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

	ciOut := runCmd(t, repo, env, julPath, "ci", "--cmd", "true", "--json")
	var ciRes struct {
		CI struct {
			Status string `json:"status"`
		} `json:"ci"`
	}
	if err := json.NewDecoder(strings.NewReader(ciOut)).Decode(&ciRes); err != nil {
		t.Fatalf("failed to decode ci output: %v", err)
	}
	if ciRes.CI.Status == "" {
		t.Fatalf("expected ci status")
	}
	head := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	note := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", head)
	if !strings.Contains(note, "\"status\"") {
		t.Fatalf("expected attestation note, got %s", note)
	}
}
