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

	reviewOut := runCmd(t, repo, env, julPath, "review", "--suggest", "--json")
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
	var suggestions struct {
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
			Status       string `json:"status"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(strings.NewReader(suggestionsOut)).Decode(&suggestions); err != nil {
		t.Fatalf("failed to decode suggestions output: %v", err)
	}
	if len(suggestions.Suggestions) == 0 {
		t.Fatalf("expected suggestions to be stored")
	}
	if suggestions.Suggestions[0].Status == "" {
		t.Fatalf("expected suggestion status")
	}
}

func TestReviewSummaryReturnsReviewID(t *testing.T) {
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

	agentPath := filepath.Join(tmp, "agent-summary.sh")
	agentScript := `#!/bin/sh
set -e
if [ -n "$JUL_AGENT_INPUT" ]; then
  cat "$JUL_AGENT_INPUT" >/dev/null
else
  cat >/dev/null
fi
if [ -n "$JUL_AGENT_OUTPUT" ]; then
  printf '{"version":1,"status":"completed","summary":"summary from agent"}\n' > "$JUL_AGENT_OUTPUT"
else
  printf '{"version":1,"status":"completed","summary":"summary from agent"}\n'
fi
`
	if err := os.WriteFile(agentPath, []byte(agentScript), 0o755); err != nil {
		t.Fatalf("write agent script failed: %v", err)
	}

	julPath := buildCLI(t)
	home := filepath.Join(tmp, "home")
	env := map[string]string{
		"HOME":           home,
		"JUL_WORKSPACE":  "tester/@",
		"JUL_AGENT_CMD":  agentPath,
		"JUL_AGENT_MODE": "file",
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review")

	reviewOut := runCmd(t, repo, env, julPath, "review", "--json")
	var reviewRes struct {
		Review *struct {
			ReviewID string `json:"review_id"`
			Summary  string `json:"summary"`
		} `json:"review"`
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(strings.NewReader(reviewOut)).Decode(&reviewRes); err != nil {
		t.Fatalf("failed to decode review output: %v", err)
	}
	if reviewRes.Review == nil || reviewRes.Review.ReviewID == "" {
		t.Fatalf("expected review_id in summary output")
	}
	if reviewRes.Review.Summary == "" {
		t.Fatalf("expected review summary text")
	}
	if len(reviewRes.Suggestions) != 0 {
		t.Fatalf("expected no suggestions in summary output")
	}

	suggestionsOut := runCmd(t, repo, env, julPath, "suggestions", "--json")
	var suggestions struct {
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(strings.NewReader(suggestionsOut)).Decode(&suggestions); err != nil {
		t.Fatalf("failed to decode suggestions output: %v", err)
	}
	if len(suggestions.Suggestions) != 0 {
		t.Fatalf("expected no suggestions stored for summary review")
	}
}

func TestReviewSuggestFromUsesSummary(t *testing.T) {
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

	summaryPath := filepath.Join(tmp, "agent-summary.sh")
	summaryScript := `#!/bin/sh
set -e
if [ -n "$JUL_AGENT_INPUT" ]; then
  cat "$JUL_AGENT_INPUT" >/dev/null
else
  cat >/dev/null
fi
if [ -n "$JUL_AGENT_OUTPUT" ]; then
  printf '{"version":1,"status":"completed","summary":"prior summary"}\n' > "$JUL_AGENT_OUTPUT"
else
  printf '{"version":1,"status":"completed","summary":"prior summary"}\n'
fi
`
	if err := os.WriteFile(summaryPath, []byte(summaryScript), 0o755); err != nil {
		t.Fatalf("write agent script failed: %v", err)
	}

	suggestPath := filepath.Join(tmp, "agent-suggest.sh")
	suggestScript := `#!/bin/sh
set -e
if [ -z "$JUL_AGENT_INPUT" ]; then
  echo "missing JUL_AGENT_INPUT" >&2
  exit 1
fi
if ! grep -q '"prior_summary":"prior summary"' "$JUL_AGENT_INPUT"; then
  echo "missing prior summary" >&2
  exit 1
fi
cd "$JUL_AGENT_WORKSPACE"
git config user.name "Agent"
git config user.email "agent@example.com"
echo "agent change" >> README.md
git add README.md
git commit -m "agent suggestion" >/dev/null
sha=$(git rev-parse HEAD)
if [ -n "$JUL_AGENT_OUTPUT" ]; then
  printf '{"version":1,"status":"completed","suggestions":[{"commit":"%s","reason":"review","description":"agent change","confidence":0.9}]}\n' "$sha" > "$JUL_AGENT_OUTPUT"
else
  printf '{"version":1,"status":"completed","suggestions":[{"commit":"%s","reason":"review","description":"agent change","confidence":0.9}]}\n' "$sha"
fi
`
	if err := os.WriteFile(suggestPath, []byte(suggestScript), 0o755); err != nil {
		t.Fatalf("write agent script failed: %v", err)
	}

	julPath := buildCLI(t)
	home := filepath.Join(tmp, "home")
	envSummary := map[string]string{
		"HOME":           home,
		"JUL_WORKSPACE":  "tester/@",
		"JUL_AGENT_CMD":  summaryPath,
		"JUL_AGENT_MODE": "file",
	}
	envSuggest := map[string]string{
		"HOME":           home,
		"JUL_WORKSPACE":  "tester/@",
		"JUL_AGENT_CMD":  suggestPath,
		"JUL_AGENT_MODE": "file",
	}

	runCmd(t, repo, envSummary, julPath, "init", "demo")
	runCmd(t, repo, envSummary, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review")

	reviewOut := runCmd(t, repo, envSummary, julPath, "review", "--json")
	var summaryRes struct {
		Review struct {
			ReviewID string `json:"review_id"`
		} `json:"review"`
	}
	if err := json.NewDecoder(strings.NewReader(reviewOut)).Decode(&summaryRes); err != nil {
		t.Fatalf("failed to decode summary output: %v", err)
	}
	if summaryRes.Review.ReviewID == "" {
		t.Fatalf("expected review_id in summary output")
	}

	suggestOut := runCmd(t, repo, envSuggest, julPath, "review", "--suggest", "--from", summaryRes.Review.ReviewID, "--json")
	var suggestRes struct {
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(strings.NewReader(suggestOut)).Decode(&suggestRes); err != nil {
		t.Fatalf("failed to decode suggest output: %v", err)
	}
	if len(suggestRes.Suggestions) == 0 || suggestRes.Suggestions[0].SuggestionID == "" {
		t.Fatalf("expected suggestions from review --suggest --from")
	}
}
