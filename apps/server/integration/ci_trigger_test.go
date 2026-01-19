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

func TestCITriggerRuns(t *testing.T) {
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
	runCmd(t, repo, nil, "go", "mod", "init", "example.com/demo")
	writeFile(t, repo, "demo.go", "package demo\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, repo, "demo_test.go", "package demo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1,2)!=3 { t.Fatalf(\"bad\") } }\n")
	runCmd(t, repo, nil, "git", "add", "go.mod", "demo.go", "demo_test.go")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: ci")
	commitSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))

	runCmd(t, repo, nil, "git", "push", "origin", "HEAD:main")

	workspaceID := "tester/workspace"
	syncPayload := storage.SyncPayload{
		WorkspaceID: workspaceID,
		Repo:        repoName,
		Branch:      "main",
		CommitSHA:   commitSHA,
		ChangeID:    "",
		Message:     "feat: ci",
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
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync expected 200, got %d", resp.StatusCode)
	}

	triggerPayload := map[string]any{
		"commit_sha": commitSHA,
		"profile":    "unit",
	}
	body, _ = json.Marshal(triggerPayload)
	req, err = http.NewRequest(http.MethodPost, baseURL+"/api/v1/ci/trigger", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("trigger request failed: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("trigger failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var att struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&att); err != nil {
		t.Fatalf("failed to decode attestation: %v", err)
	}
	if att.Status != "pass" {
		t.Fatalf("expected pass, got %s", att.Status)
	}
}
