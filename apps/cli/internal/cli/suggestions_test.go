package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/metadata"
)

func TestSuggestionsPendingRefreshesLiveDraftContext(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	writeFilePath(t, repo, "README.md", "one\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "base")
	baseSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	writeFilePath(t, repo, "README.md", "two\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "older draft")
	olderDraftSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	writeFilePath(t, repo, "README.md", "three\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "current draft")
	currentDraftSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	t.Setenv("HOME", filepath.Join(repo, "home"))
	t.Setenv("JUL_WORKSPACE", "tester/@")

	changeID := "I3333333333333333333333333333333333333333"
	if _, err := metadata.CreateSuggestion(metadata.SuggestionCreate{
		ChangeID:           changeID,
		BaseCommitSHA:      currentDraftSHA,
		SuggestedCommitSHA: currentDraftSHA,
		CreatedBy:          "tester",
		Reason:             "regression",
	}); err != nil {
		t.Fatalf("CreateSuggestion failed: %v", err)
	}

	cachePayload := map[string]any{
		"workspace": "@",
		"draft_sha": olderDraftSHA,
		"change_id": changeID,
		"last_checkpoint": map[string]any{
			"commit_sha": baseSHA,
			"message":    "stale cache checkpoint",
		},
	}
	cacheData, err := json.Marshal(cachePayload)
	if err != nil {
		t.Fatalf("failed to marshal status cache payload: %v", err)
	}
	cachePath := filepath.Join(repo, ".jul", "status.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, cacheData, 0o644); err != nil {
		t.Fatalf("failed to write status cache: %v", err)
	}

	out := captureStdout(t, func() int {
		return newSuggestionsCommand().Run([]string{
			"--json",
			"--change-id", changeID,
			"--status", "pending",
			"--limit", "20",
		})
	})

	var payload struct {
		Suggestions []json.RawMessage `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("failed to decode suggestions output: %v\n%s", err, out)
	}
	if len(payload.Suggestions) != 1 {
		t.Fatalf("expected 1 fresh pending suggestion, got %d (%s)", len(payload.Suggestions), out)
	}
}
