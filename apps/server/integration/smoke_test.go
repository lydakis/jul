package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type reflogEntry struct {
	CommitSHA string `json:"commit_sha"`
	ChangeID  string `json:"change_id"`
	Source    string `json:"source"`
}

func TestSmokeSyncAndReflog(t *testing.T) {
	baseURL, cleanup := startServer(t, "")
	defer cleanup()

	repo := t.TempDir()
	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	julPath := buildCLI(t)
	workspaceID := "tester/workspace"
	env := map[string]string{
		"JUL_BASE_URL":  baseURL,
		"JUL_WORKSPACE": workspaceID,
	}

	// Install hook
	runCmd(t, repo, env, julPath, "hooks", "install")
	hooksDir := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "--git-path", "hooks"))
	if !filepath.IsAbs(hooksDir) {
		hooksDir = filepath.Join(repo, hooksDir)
	}
	hookPath := filepath.Join(hooksDir, "post-commit")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("expected hook at %s: %v", hookPath, err)
	}

	// Commit 1
	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: first")
	sha1 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, env, julPath, "sync")

	// Commit 2
	writeFile(t, repo, "README.md", "hello\nworld\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: second")
	sha2 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, env, julPath, "sync")

	// Record a CI attestation
	runCmd(t, repo, env, julPath, "ci", "run", "--cmd", "true")

	// CLI reflog should include latest commit
	reflogOut := runCmd(t, repo, env, julPath, "reflog", "--limit", "5")
	if !strings.Contains(reflogOut, sha2) {
		t.Fatalf("expected reflog output to contain latest commit %s", sha2)
	}

	// API reflog should return current + keep entries
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/workspaces/%s/reflog?limit=10", baseURL, workspaceID), nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reflog request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var entries []reflogEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode reflog: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
	if entries[0].CommitSHA != sha2 || entries[0].Source != "current" {
		t.Fatalf("expected current entry %s, got %s (%s)", sha2, entries[0].CommitSHA, entries[0].Source)
	}
	foundKeep := false
	for _, entry := range entries {
		if entry.CommitSHA == sha1 && entry.Source == "keep" {
			foundKeep = true
			break
		}
	}
	if !foundKeep {
		t.Fatalf("expected keep entry for %s", sha1)
	}

	attReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/commits/%s/attestation", baseURL, sha2), nil)
	if err != nil {
		t.Fatalf("failed to build attestation request: %v", err)
	}
	attResp, err := http.DefaultClient.Do(attReq)
	if err != nil {
		t.Fatalf("attestation request failed: %v", err)
	}
	defer attResp.Body.Close()
	if attResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", attResp.StatusCode)
	}
	var att struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(attResp.Body).Decode(&att); err != nil {
		t.Fatalf("failed to decode attestation: %v", err)
	}
	if att.Status != "pass" {
		t.Fatalf("expected attestation status pass, got %s", att.Status)
	}

	queryReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/query?tests=pass&limit=5", baseURL), nil)
	if err != nil {
		t.Fatalf("failed to build query request: %v", err)
	}
	queryResp, err := http.DefaultClient.Do(queryReq)
	if err != nil {
		t.Fatalf("query request failed: %v", err)
	}
	defer queryResp.Body.Close()
	if queryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", queryResp.StatusCode)
	}
	var queryResults []struct {
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(queryResp.Body).Decode(&queryResults); err != nil {
		t.Fatalf("failed to decode query: %v", err)
	}
	found := false
	for _, res := range queryResults {
		if res.CommitSHA == sha2 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected query to include %s", sha2)
	}
}
