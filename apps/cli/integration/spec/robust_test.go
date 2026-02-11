//go:build jul_integ_spec

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIT_ROBUST_002(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	runCmd(t, repo, device.Env, julPath, "sync", "--json")

	deviceID := readDeviceID(t, device.Home)
	workspaceRef := "refs/jul/workspaces/tester/@"
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"

	workspaceBefore := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", workspaceRef))
	syncBefore := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", syncRef))
	keepBefore := runCmd(t, repo, nil, "git", "for-each-ref", "--format=%(refname)", "refs/jul/keep/tester/@/")
	changeBefore := runCmd(t, repo, nil, "git", "for-each-ref", "--format=%(refname)", "refs/jul/changes/")
	pauseMarker := filepath.Join(repo, ".jul", "checkpoint-pause-marker")

	cmd := exec.Command(julPath, "checkpoint", "-m", "interrupted checkpoint", "--no-ci", "--no-review", "--json")
	cmd.Dir = repo
	cmd.Env = mergeEnv(map[string]string{
		"HOME":                device.Home,
		"XDG_CONFIG_HOME":     device.XDG,
		"JUL_WORKSPACE":       "tester/@",
		"OPENCODE_PERMISSION": `{"*":"allow"}`,
		"JUL_TEST_CHECKPOINT_PAUSE_BEFORE_REFS_MS": "5000",
		"JUL_TEST_CHECKPOINT_PAUSE_MARKER":         pauseMarker,
	})
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start checkpoint command: %v", err)
	}
	waitForFile(t, pauseMarker, 10*time.Second)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to interrupt checkpoint command: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatalf("expected checkpoint to be interrupted, got success: %s", out.String())
	}

	workspaceAfter := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", workspaceRef))
	syncAfter := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", syncRef))
	keepAfter := runCmd(t, repo, nil, "git", "for-each-ref", "--format=%(refname)", "refs/jul/keep/tester/@/")
	changeAfter := runCmd(t, repo, nil, "git", "for-each-ref", "--format=%(refname)", "refs/jul/changes/")

	if workspaceAfter != workspaceBefore {
		t.Fatalf("expected workspace ref unchanged, got %s vs %s", workspaceAfter, workspaceBefore)
	}
	if syncAfter != syncBefore {
		t.Fatalf("expected sync ref unchanged, got %s vs %s", syncAfter, syncBefore)
	}
	if keepAfter != keepBefore {
		t.Fatalf("expected keep refs unchanged, got before=%q after=%q", keepBefore, keepAfter)
	}
	if changeAfter != changeBefore {
		t.Fatalf("expected change refs unchanged, got before=%q after=%q", changeBefore, changeAfter)
	}

	logOut := runCmd(t, repo, device.Env, julPath, "log", "--limit", "1", "--json")
	var entries []any
	if err := json.NewDecoder(strings.NewReader(logOut)).Decode(&entries); err != nil {
		t.Fatalf("failed to decode log output: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no checkpoint entries after interrupted checkpoint, got %d", len(entries))
	}
}
