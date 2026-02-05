package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewWatchDetachesAndCompletes(t *testing.T) {
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
sleep 1
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

	reviewDir := filepath.Join(repo, ".jul", "review")
	_ = os.RemoveAll(reviewDir)

	cmd := exec.Command(julPath, "review", "--watch", "--json")
	cmd.Dir = repo
	cmd.Env = mergeEnv(env)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		t.Fatalf("start review failed: %v", err)
	}

	logDir := filepath.Join(reviewDir, "logs")
	_ = waitForSuffixFile(t, logDir, ".log", 2*time.Second)

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to interrupt review: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("review did not detach cleanly: %v\n%s", err, output.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("review did not exit after interrupt")
	}

	resultDir := filepath.Join(reviewDir, "results")
	resultPath := waitForSuffixFile(t, resultDir, ".json", 4*time.Second)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read review result failed: %v", err)
	}
	var res struct {
		Summary string `json:"summary"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatalf("decode review result failed: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("review error: %s", res.Error)
	}
	if res.Summary == "" {
		t.Fatalf("expected review summary in result")
	}
}

func TestCheckpointStartsBackgroundCIAndReview(t *testing.T) {
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

	agentPath := filepath.Join(tmp, "agent-suggest.sh")
	agentScript := `#!/bin/sh
set -e
sleep 1
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

	ciConfig := `[commands]
cmd1 = "sleep 1"
`
	if err := os.MkdirAll(filepath.Join(repo, ".jul"), 0o755); err != nil {
		t.Fatalf("failed to create .jul dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".jul", "ci.toml"), []byte(ciConfig), 0o644); err != nil {
		t.Fatalf("write ci config failed: %v", err)
	}

	_ = os.RemoveAll(filepath.Join(repo, ".jul", "ci"))
	_ = os.RemoveAll(filepath.Join(repo, ".jul", "review"))

	runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first")

	ciRunPath := waitForSuffixFile(t, filepath.Join(repo, ".jul", "ci", "runs"), ".json", 5*time.Second)
	waitForCIRunCompletion(t, ciRunPath, 5*time.Second)

	reviewResult := waitForSuffixFile(t, filepath.Join(repo, ".jul", "review", "results"), ".json", 5*time.Second)
	data, err := os.ReadFile(reviewResult)
	if err != nil {
		t.Fatalf("read review result failed: %v", err)
	}
	var res struct {
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
		} `json:"suggestions"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatalf("decode review result failed: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("review error: %s", res.Error)
	}
	if len(res.Suggestions) == 0 {
		t.Fatalf("expected background review suggestions")
	}
}

func waitForSuffixFile(t *testing.T, dir, suffix string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if filepath.Ext(entry.Name()) == suffix {
					return filepath.Join(dir, entry.Name())
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s file in %s", suffix, dir)
	return ""
}

func waitForCIRunCompletion(t *testing.T, runPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(runPath)
		if err != nil {
			t.Fatalf("read ci run failed: %v", err)
		}
		var run struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(data, &run); err != nil {
			t.Fatalf("decode ci run failed: %v", err)
		}
		if run.Status != "" && run.Status != "running" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("ci run did not complete in time")
}
