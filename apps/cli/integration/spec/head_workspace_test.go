//go:build jul_integ_spec

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIT_HEAD_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, device.Env, julPath, "sync", "--json")
	assertHeadRef(t, repo, "refs/heads/jul/@")

	checkpointOut := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: one", "--no-ci", "--no-review", "--json")
	if !strings.Contains(checkpointOut, "CheckpointSHA") {
		t.Fatalf("expected checkpoint json output, got %s", checkpointOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")

	traceOut := runCmd(t, repo, device.Env, julPath, "trace", "--prompt", "trace", "--json")
	if !strings.Contains(traceOut, "trace_sha") {
		t.Fatalf("expected trace json output, got %s", traceOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")

	agentPath := filepath.Join(t.TempDir(), "agent.sh")
	agentScript := `#!/bin/sh
set -e
cd "$JUL_AGENT_WORKSPACE"
git config user.name "Agent"
git config user.email "agent@example.com"
echo "agent change" >> README.md
git add README.md
git commit -m "agent suggestion" >/dev/null
sha=$(git rev-parse HEAD)
printf '{"version":1,"status":"completed","suggestions":[{"commit":"%s","reason":"review","description":"agent change","confidence":0.9}]}' "$sha"
`
	if err := os.WriteFile(agentPath, []byte(agentScript), 0o755); err != nil {
		t.Fatalf("write agent script failed: %v", err)
	}
	envAgent := map[string]string{
		"HOME":          device.Home,
		"XDG_CONFIG_HOME": device.XDG,
		"JUL_WORKSPACE": "tester/@",
		"JUL_AGENT_CMD": agentPath,
	}

	reviewOut := runCmd(t, repo, envAgent, julPath, "review", "--json")
	if !strings.Contains(reviewOut, "suggestions") {
		t.Fatalf("expected review json output, got %s", reviewOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")

	statusOut := runCmd(t, repo, device.Env, julPath, "status", "--json")
	if !strings.Contains(statusOut, "workspace_id") {
		t.Fatalf("expected status json output, got %s", statusOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
}

func assertHeadRef(t *testing.T, repo, want string) {
	t.Helper()
	head := strings.TrimSpace(runCmd(t, repo, nil, "git", "symbolic-ref", "HEAD"))
	if head != want {
		t.Fatalf("expected HEAD %s, got %s", want, head)
	}
}
