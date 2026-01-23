package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewCreatesSuggestion(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: base")

	agentPath := filepath.Join(tmp, "agent.sh")
	agentScript := `#!/bin/sh
set -e
cd "$JUL_AGENT_WORKSPACE"
git config user.name "Agent"
git config user.email "agent@example.com"
echo "agent change" >> README.md
git add README.md
git commit -m "agent suggestion" >/dev/null
sha=$(git rev-parse HEAD)
printf '{"version":1,"status":"completed","suggestions":[{"commit":"%s","reason":"review","description":"agent change","confidence":0.9}]}\n' "$sha"
`
	if err := os.WriteFile(agentPath, []byte(agentScript), 0o755); err != nil {
		t.Fatalf("write agent script failed: %v", err)
	}

	julPath := buildCLI(t)
	home := filepath.Join(tmp, "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
		"JUL_AGENT_CMD": agentPath,
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review")

	reviewOut := runCmd(t, repo, env, julPath, "review", "--json")
	var reviewRes struct {
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(strings.NewReader(reviewOut)).Decode(&reviewRes); err != nil {
		t.Fatalf("failed to decode review output: %v", err)
	}
	if len(reviewRes.Suggestions) == 0 || reviewRes.Suggestions[0].SuggestionID == "" {
		t.Fatalf("expected suggestions in review output")
	}

	suggestionsOut := runCmd(t, repo, env, julPath, "suggestions", "--json")
	var suggestions []struct {
		SuggestionID string `json:"suggestion_id"`
		Status       string `json:"status"`
	}
	if err := json.NewDecoder(strings.NewReader(suggestionsOut)).Decode(&suggestions); err != nil {
		t.Fatalf("failed to decode suggestions output: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatalf("expected suggestions to be stored")
	}
	if suggestions[0].Status == "" {
		t.Fatalf("expected suggestion status")
	}
}
