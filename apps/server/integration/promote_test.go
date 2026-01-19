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

func TestPromotionUpdatesRef(t *testing.T) {
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
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: promote")
	commitSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))

	runCmd(t, repo, nil, "git", "push", "origin", "HEAD:main")

	workspaceID := "tester/workspace"
	syncPayload := storage.SyncPayload{
		WorkspaceID: workspaceID,
		Repo:        repoName,
		Branch:      "main",
		CommitSHA:   commitSHA,
		ChangeID:    "",
		Message:     "feat: promote",
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

	promotePayload := map[string]any{
		"target_branch": "release",
		"commit_sha":    commitSHA,
		"force":         true,
	}
	body, _ = json.Marshal(promotePayload)
	req, err = http.NewRequest(http.MethodPost, baseURL+"/api/v1/workspaces/"+workspaceID+"/promote", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("promote request failed: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("promote expected 200, got %d", resp.StatusCode)
	}

	refSHA := strings.TrimSpace(runCmd(t, repoRoot, nil, "git", "--git-dir", bareRepo, "rev-parse", "refs/heads/release"))
	if refSHA != commitSHA {
		t.Fatalf("expected ref %s, got %s", commitSHA, refSHA)
	}
}
