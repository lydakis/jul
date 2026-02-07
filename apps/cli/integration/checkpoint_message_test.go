package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckpointGeneratesMessageWhenMissing(t *testing.T) {
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

	agentPath := filepath.Join(tmp, "agent-message.sh")
	agentScript := `#!/bin/sh
set -e
if [ -z "$JUL_AGENT_INPUT" ]; then
  echo "missing JUL_AGENT_INPUT" >&2
  exit 1
fi
if ! grep -q '"action":"generate_message"' "$JUL_AGENT_INPUT"; then
  echo "missing generate_message action" >&2
  cat "$JUL_AGENT_INPUT" >&2
  exit 1
fi
if [ -n "$JUL_AGENT_OUTPUT" ]; then
  printf '{"version":1,"status":"completed","summary":"feat: generated checkpoint message"}\n' > "$JUL_AGENT_OUTPUT"
else
  printf '{"version":1,"status":"completed","summary":"feat: generated checkpoint message"}\n'
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
	out := runCmd(t, repo, env, julPath, "checkpoint", "--no-ci", "--no-review", "--json")
	var checkpoint struct {
		CheckpointSHA string
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&checkpoint); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if checkpoint.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	message := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", checkpoint.CheckpointSHA)
	if !strings.Contains(message, "feat: generated checkpoint message") {
		t.Fatalf("expected generated checkpoint message, got %q", message)
	}
}

func TestCheckpointFailsWhenAgentMessageGenerationFails(t *testing.T) {
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
	headBefore := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))

	agentPath := filepath.Join(tmp, "agent-fail.sh")
	agentScript := `#!/bin/sh
echo "agent failed" >&2
exit 2
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
	out, err := runCmdAllowFailure(t, repo, env, julPath, "checkpoint", "--no-ci", "--no-review", "--json")
	if err == nil {
		t.Fatalf("expected checkpoint to fail when message generation fails")
	}
	if !strings.Contains(out, "failed to generate checkpoint message") {
		t.Fatalf("expected message generation error, got %s", out)
	}

	headAfter := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	if headAfter != headBefore {
		t.Fatalf("expected HEAD unchanged on message generation failure")
	}
}

func TestCheckpointGeneratedMessageStripsReservedTrailers(t *testing.T) {
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

	agentPath := filepath.Join(tmp, "agent-trailers.sh")
	agentScript := `#!/bin/sh
set -e
payload='{"version":1,"status":"completed","summary":"feat: generated checkpoint message\n\nChange-Id: I1111111111111111111111111111111111111111\nTrace-Head: deadbeef\nTrace-Base: badbase"}'
if [ -n "$JUL_AGENT_OUTPUT" ]; then
  printf '%s\n' "$payload" > "$JUL_AGENT_OUTPUT"
else
  printf '%s\n' "$payload"
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
	out := runCmd(t, repo, env, julPath, "checkpoint", "--no-ci", "--no-review", "--json")
	var checkpoint struct {
		CheckpointSHA string
		ChangeID      string
		TraceHead     string
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&checkpoint); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if checkpoint.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}
	if checkpoint.ChangeID == "" {
		t.Fatalf("expected canonical change id")
	}
	if checkpoint.TraceHead == "" {
		t.Fatalf("expected canonical trace head")
	}

	message := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", checkpoint.CheckpointSHA)
	if strings.Contains(message, "Change-Id: I1111111111111111111111111111111111111111") {
		t.Fatalf("expected generated change-id trailer to be removed, got %q", message)
	}
	if strings.Contains(message, "Trace-Head: deadbeef") {
		t.Fatalf("expected generated trace-head trailer to be removed, got %q", message)
	}
	if strings.Contains(message, "Trace-Base: badbase") {
		t.Fatalf("expected generated trace-base trailer to be removed, got %q", message)
	}
	if !strings.Contains(message, "Change-Id: "+checkpoint.ChangeID) {
		t.Fatalf("expected canonical change-id trailer in message, got %q", message)
	}
	if !strings.Contains(message, "Trace-Head: "+checkpoint.TraceHead) {
		t.Fatalf("expected canonical trace-head trailer in message, got %q", message)
	}
}
