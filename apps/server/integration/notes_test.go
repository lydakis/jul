package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lydakis/jul/server/internal/storage"
)

func TestAttestationNotesWritten(t *testing.T) {
	repoRoot := t.TempDir()
	repoName := "demo"

	bareRepo := filepath.Join(repoRoot, repoName+".git")
	runCmd(t, repoRoot, nil, "git", "init", "--bare", bareRepo)

	baseURL, cleanup := startServer(t, repoRoot)
	defer cleanup()

	repo := t.TempDir()
	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")
	runCmd(t, repo, nil, "git", "remote", "add", "origin", bareRepo)

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: notes")
	commitSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))

	runCmd(t, repo, nil, "git", "push", "origin", "HEAD:main")

	workspaceID := "tester/workspace"
	syncPayload := storage.SyncPayload{
		WorkspaceID: workspaceID,
		Repo:        repoName,
		Branch:      "main",
		CommitSHA:   commitSHA,
		ChangeID:    "",
		Message:     "feat: notes",
		Author:      "Test User",
		CommittedAt: time.Now().UTC(),
	}

	body, _ := json.Marshal(syncPayload)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/sync", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("sync request failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync expected 200, got %d", resp.StatusCode)
	}
	var syncRes storage.SyncResult
	if err := json.NewDecoder(resp.Body).Decode(&syncRes); err != nil {
		t.Fatalf("failed to decode sync response: %v", err)
	}
	_ = resp.Body.Close()

	attPayload := map[string]any{
		"commit_sha":   commitSHA,
		"change_id":    syncRes.Change.ChangeID,
		"type":         "ci",
		"status":       "pass",
		"started_at":   time.Now().UTC(),
		"finished_at":  time.Now().UTC(),
		"signals_json": "{}",
	}
	body, _ = json.Marshal(attPayload)
	req, err = http.NewRequest(http.MethodPost, baseURL+"/api/v1/attestations", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("attestation request failed: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("attestation failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("attestation expected 201, got %d", resp.StatusCode)
	}

	note := strings.TrimSpace(runCmd(t, repoRoot, nil, "git", "--git-dir", bareRepo, "notes", "--ref", "refs/notes/jul/attestations", "show", commitSHA))
	if !strings.Contains(note, commitSHA) || !strings.Contains(note, "attestation_id") {
		t.Fatalf("expected note to contain commit sha and attestation id, got: %s", note)
	}
}
