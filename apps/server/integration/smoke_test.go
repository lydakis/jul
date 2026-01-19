package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type reflogEntry struct {
	CommitSHA string `json:"commit_sha"`
	ChangeID  string `json:"change_id"`
	Source    string `json:"source"`
}

func TestSmokeSyncAndReflog(t *testing.T) {
	reposDir := filepath.Join(t.TempDir(), "repos")
	baseURL, cleanup := startServer(t, reposDir)
	defer cleanup()

	bareRepo := filepath.Join(reposDir, "demo.git")
	runCmd(t, reposDir, nil, "git", "init", "--bare", bareRepo)

	repo := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "branch", "-M", "main")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")
	runCmd(t, repo, nil, "git", "remote", "add", "origin", bareRepo)

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
	runCmd(t, repo, nil, "git", "push", "-u", "origin", "main")
	runCmd(t, repo, env, julPath, "sync")

	// Commit 2
	writeFile(t, repo, "README.md", "hello\nworld\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: second")
	sha2 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, nil, "git", "push", "origin", "main")
	runCmd(t, repo, env, julPath, "sync")

	// Record a CI attestation
	runCmd(t, repo, env, julPath, "ci", "run", "--cmd", "true", "--coverage-line", "85")

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

	commitTimeRaw := strings.TrimSpace(runCmd(t, repo, nil, "git", "log", "-1", "--format=%cI"))
	commitTime, err := time.Parse(time.RFC3339, commitTimeRaw)
	if err != nil {
		t.Fatalf("failed to parse commit time: %v", err)
	}
	since := commitTime.Add(-1 * time.Minute).Format(time.RFC3339)
	until := commitTime.Add(1 * time.Minute).Format(time.RFC3339)

	queryReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/query?tests=pass&compiles=true&coverage_min=80&since=%s&until=%s&limit=5", baseURL, since, until), nil)
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

	// Commit 3 (suggestion)
	writeFile(t, repo, "README.md", "hello\nworld\nsuggestion\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "fix: suggestion")
	sha3 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, nil, "git", "push", "origin", "main")

	commitReq, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/commits/%s", baseURL, sha2), nil)
	if err != nil {
		t.Fatalf("failed to build commit request: %v", err)
	}
	commitResp, err := http.DefaultClient.Do(commitReq)
	if err != nil {
		t.Fatalf("commit request failed: %v", err)
	}
	defer commitResp.Body.Close()
	if commitResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", commitResp.StatusCode)
	}
	var commitInfo struct {
		ChangeID string `json:"change_id"`
	}
	if err := json.NewDecoder(commitResp.Body).Decode(&commitInfo); err != nil {
		t.Fatalf("failed to decode commit response: %v", err)
	}
	if commitInfo.ChangeID == "" {
		t.Fatalf("expected change_id in commit response")
	}

	suggestionBody, err := json.Marshal(map[string]any{
		"change_id":            commitInfo.ChangeID,
		"base_commit_sha":      sha2,
		"suggested_commit_sha": sha3,
		"reason":               "fix_tests",
		"description":          "example suggestion",
		"confidence":           0.8,
		"diffstat": map[string]int{
			"files_changed": 1,
			"additions":     1,
			"deletions":     0,
		},
	})
	if err != nil {
		t.Fatalf("failed to encode suggestion body: %v", err)
	}

	suggestionReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/suggestions", baseURL), bytes.NewReader(suggestionBody))
	if err != nil {
		t.Fatalf("failed to build suggestion request: %v", err)
	}
	suggestionReq.Header.Set("Content-Type", "application/json")
	suggestionResp, err := http.DefaultClient.Do(suggestionReq)
	if err != nil {
		t.Fatalf("suggestion request failed: %v", err)
	}
	defer suggestionResp.Body.Close()
	if suggestionResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", suggestionResp.StatusCode)
	}
	var suggestion struct {
		SuggestionID string `json:"suggestion_id"`
	}
	if err := json.NewDecoder(suggestionResp.Body).Decode(&suggestion); err != nil {
		t.Fatalf("failed to decode suggestion response: %v", err)
	}
	if suggestion.SuggestionID == "" {
		t.Fatalf("expected suggestion_id")
	}

	ref := fmt.Sprintf("refs/jul/suggest/%s/%s", commitInfo.ChangeID, suggestion.SuggestionID)
	refOut := strings.TrimSpace(runCmd(t, repo, nil, "git", "--git-dir", bareRepo, "show-ref", ref))
	if !strings.Contains(refOut, sha3) {
		t.Fatalf("expected suggestion ref to point at %s, got %s", sha3, refOut)
	}

	acceptReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/suggestions/%s/accept", baseURL, suggestion.SuggestionID), nil)
	if err != nil {
		t.Fatalf("failed to build accept request: %v", err)
	}
	acceptResp, err := http.DefaultClient.Do(acceptReq)
	if err != nil {
		t.Fatalf("accept request failed: %v", err)
	}
	defer acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", acceptResp.StatusCode)
	}
}
