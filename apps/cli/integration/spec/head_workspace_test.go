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

	headInit := headSHA(t, repo)
	runCmd(t, repo, device.Env, julPath, "sync", "--json")
	assertHeadRef(t, repo, "refs/heads/jul/@")
	assertHeadUnchanged(t, repo, headInit, "sync")

	writeFile(t, repo, "README.md", "hello\n")
	checkpointOut := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: one", "--no-ci", "--no-review", "--json")
	if !strings.Contains(checkpointOut, "CheckpointSHA") {
		t.Fatalf("expected checkpoint json output, got %s", checkpointOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
	headAfterCheckpoint := headSHA(t, repo)
	if headAfterCheckpoint == headInit {
		t.Fatalf("expected HEAD to move after checkpoint")
	}

	traceOut := runCmd(t, repo, device.Env, julPath, "trace", "--prompt", "trace", "--json")
	if !strings.Contains(traceOut, "trace_sha") {
		t.Fatalf("expected trace json output, got %s", traceOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
	assertHeadUnchanged(t, repo, headAfterCheckpoint, "trace")

	reviewOut := runReview(t, repo, device.Env, julPath)
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
	assertHeadUnchanged(t, repo, headAfterCheckpoint, "review")

	statusOut := runCmd(t, repo, device.Env, julPath, "status", "--json")
	if !strings.Contains(statusOut, "workspace_id") {
		t.Fatalf("expected status json output, got %s", statusOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
	assertHeadUnchanged(t, repo, headAfterCheckpoint, "status")

	runCmd(t, repo, device.Env, julPath, "ws", "new", "feature")
	assertHeadRef(t, repo, "refs/heads/jul/feature")
	runCmd(t, repo, device.Env, julPath, "ws", "switch", "@")
	assertHeadRef(t, repo, "refs/heads/jul/@")
	assertHeadUnchanged(t, repo, headAfterCheckpoint, "ws switch")

	writeFile(t, repo, "README.md", "hello\nsecond\n")
	checkpointOut2 := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: two", "--no-ci", "--no-review", "--json")
	if !strings.Contains(checkpointOut2, "CheckpointSHA") {
		t.Fatalf("expected checkpoint json output, got %s", checkpointOut2)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
	headAfterSecond := headSHA(t, repo)
	if headAfterSecond == headAfterCheckpoint {
		t.Fatalf("expected HEAD to move after second checkpoint")
	}

	runCmd(t, repo, nil, "git", "switch", "main")
	writeFile(t, repo, "base.txt", "base\n")
	runCmd(t, repo, nil, "git", "add", "base.txt")
	runCmd(t, repo, nil, "git", "commit", "-m", "base advance")
	runCmd(t, repo, device.Env, julPath, "ws", "checkout", "@")
	assertHeadRef(t, repo, "refs/heads/jul/@")
	workspaceTip := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "refs/jul/workspaces/tester/@"))
	if headSHA(t, repo) != workspaceTip {
		t.Fatalf("expected HEAD to match workspace ref %s after ws checkout, got %s", workspaceTip, headSHA(t, repo))
	}
	headBeforeRestack := headSHA(t, repo)

	restackOut := runCmd(t, repo, device.Env, julPath, "ws", "restack", "--json")
	if !strings.Contains(restackOut, "\"status\"") {
		t.Fatalf("expected restack json output, got %s", restackOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
	headAfterRestack := headSHA(t, repo)
	if headAfterRestack == headBeforeRestack {
		t.Fatalf("expected HEAD to move after restack")
	}

	headBeforePromote := headAfterRestack
	promoteOut := runCmd(t, repo, device.Env, julPath, "promote", "--to", "main", "--rebase", "--json")
	if !strings.Contains(promoteOut, "\"status\"") {
		t.Fatalf("expected promote json output, got %s", promoteOut)
	}
	assertHeadRef(t, repo, "refs/heads/jul/@")
	headAfterPromote := headSHA(t, repo)
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

func headSHA(t *testing.T, repo string) string {
	t.Helper()
	return strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
}

func assertHeadUnchanged(t *testing.T, repo, want, context string) {
	t.Helper()
	got := headSHA(t, repo)
	if got != want {
		t.Fatalf("expected HEAD to remain %s after %s, got %s", want, context, got)
	}
}

func runReview(t *testing.T, repo string, env map[string]string, julPath string) string {
	t.Helper()
	out, err := runCmdAllowFailure(t, repo, env, julPath, "review", "--json")
	if err == nil {
		return out
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "opencode failed") || strings.Contains(lower, "signal: killed") {
		out, err = runCmdAllowFailure(t, repo, env, julPath, "review", "--json")
		if err == nil {
			return out
		}
	}
	t.Fatalf("review failed: %v\n%s", err, out)
	return ""
}
