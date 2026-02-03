//go:build jul_integ_spec

package integration

import (
	"encoding/json"
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

	reviewOut := runCmd(t, repo, device.Env, julPath, "review", "--json")
	var reviewRes struct {
		Review struct {
			Status string `json:"status"`
		} `json:"review"`
	}
	if err := json.NewDecoder(strings.NewReader(reviewOut)).Decode(&reviewRes); err != nil {
		t.Fatalf("failed to decode review output: %v", err)
	}
	if reviewRes.Review.Status == "" {
		t.Fatalf("expected review status, got %s", reviewOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")

	statusOut := runCmd(t, repo, device.Env, julPath, "status", "--json")
	if !strings.Contains(statusOut, "workspace_id") {
		t.Fatalf("expected status json output, got %s", statusOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")

	writeFile(t, repo, "README.md", "hello\nsecond\n")
	checkpointOut2 := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: two", "--no-ci", "--no-review", "--json")
	if !strings.Contains(checkpointOut2, "CheckpointSHA") {
		t.Fatalf("expected checkpoint json output, got %s", checkpointOut2)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")

	headBeforePromote := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	promoteOut := runCmd(t, repo, device.Env, julPath, "promote", "--to", "main", "--rebase", "--json")
	if !strings.Contains(promoteOut, "\"status\"") {
		t.Fatalf("expected promote json output, got %s", promoteOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
	headAfterPromote := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	if headAfterPromote == headBeforePromote {
		t.Fatalf("expected HEAD to move after promote")
	}
	headMsg := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", headAfterPromote)
	if !strings.Contains(headMsg, "Jul-Type: workspace-base") {
		t.Fatalf("expected workspace base marker, got %s", headMsg)
	}

	runCmd(t, repo, nil, "git", "switch", "main")
	headMain := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	runCmd(t, repo, device.Env, julPath, "ws", "checkout", "@")
	assertHeadRef(t, repo, "refs/heads/jul/@")
	headAfterCheckout := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	if headAfterCheckout == headMain {
		t.Fatalf("expected HEAD to move back to jul workspace")
	}
}

func assertHeadRef(t *testing.T, repo, want string) {
	t.Helper()
	head := strings.TrimSpace(runCmd(t, repo, nil, "git", "symbolic-ref", "HEAD"))
	if head != want {
		t.Fatalf("expected HEAD %s, got %s", want, head)
	}
}
