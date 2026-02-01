//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIT_OFFLINE_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, "offline.txt", "one\n")
	cpOut := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "offline", "--no-ci", "--no-review", "--json")
	var cp checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpOut)).Decode(&cp); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if cp.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	traceOut := runCmd(t, repo, device.Env, julPath, "trace", "--prompt", "offline", "--json")
	if !strings.Contains(traceOut, "trace_sha") {
		t.Fatalf("expected trace output, got %s", traceOut)
	}

	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, repo, device.Env, julPath, "remote", "set", "origin")
	runCmd(t, repo, device.Env, julPath, "doctor")
	runCmd(t, repo, device.Env, julPath, "sync", "--json")

	// Verify keep refs and notes are pushed.
	keepRefs := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "for-each-ref", "--format=%(refname)", "refs/jul/keep")
	if strings.TrimSpace(keepRefs) == "" {
		t.Fatalf("expected keep refs on remote")
	}
	notes := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "for-each-ref", "--format=%(refname)", "refs/notes/jul")
	if strings.TrimSpace(notes) == "" {
		t.Fatalf("expected notes refs on remote")
	}
}
