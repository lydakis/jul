package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmokeJulCLIFlow(t *testing.T) {
	reposDir := filepath.Join(t.TempDir(), "repos")
	baseURL, cleanup := startServer(t, reposDir)
	defer cleanup()

	repo := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	julPath := buildCLI(t)
	env := map[string]string{
		"JUL_BASE_URL":  baseURL,
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	bareRepo := filepath.Join(reposDir, "demo.git")
	runCmd(t, repo, nil, "git", "remote", "set-url", "jul", bareRepo)
	runCmd(t, repo, nil, "git", "branch", "-M", "main")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	// Commit 1
	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: first")
	sha1 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, nil, "git", "push", "jul", "main")
	runCmd(t, repo, env, julPath, "sync")

	// Commit 2 (suggestion)
	writeFile(t, repo, "README.md", "hello\nworld\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "fix: suggestion")
	sha2 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, nil, "git", "push", "jul", "main")
	runCmd(t, repo, env, julPath, "sync")

	runCmd(t, repo, env, julPath, "ci", "run", "--cmd", "true", "--coverage-line", "85")

	queryOut := runCmd(t, repo, env, julPath, "query", "--tests", "pass", "--limit", "5", "--json")
	var queryResults []struct {
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(queryOut)).Decode(&queryResults); err != nil {
		t.Fatalf("failed to decode query output: %v", err)
	}
	if len(queryResults) == 0 {
		t.Fatalf("expected query results")
	}

	suggestOut := runCmd(t, repo, env, julPath, "suggest", "--base", sha1, "--suggested", sha2, "--reason", "fix_tests", "--json")
	var suggestion struct {
		SuggestionID string `json:"suggestion_id"`
	}
	if err := json.NewDecoder(strings.NewReader(suggestOut)).Decode(&suggestion); err != nil {
		t.Fatalf("failed to decode suggest output: %v", err)
	}
	if suggestion.SuggestionID == "" {
		t.Fatalf("expected suggestion id")
	}

	listOut := runCmd(t, repo, env, julPath, "suggestions", "--status", "open", "--json")
	var suggestions []struct {
		SuggestionID string `json:"suggestion_id"`
	}
	if err := json.NewDecoder(strings.NewReader(listOut)).Decode(&suggestions); err != nil {
		t.Fatalf("failed to decode suggestions: %v", err)
	}
	found := false
	for _, item := range suggestions {
		if item.SuggestionID == suggestion.SuggestionID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected suggestion in list")
	}

	acceptOut := runCmd(t, repo, env, julPath, "accept", "--json", suggestion.SuggestionID)
	var accepted struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(strings.NewReader(acceptOut)).Decode(&accepted); err != nil {
		t.Fatalf("failed to decode accept output: %v", err)
	}
	if accepted.Status != "accepted" {
		t.Fatalf("expected status accepted, got %s", accepted.Status)
	}
}
