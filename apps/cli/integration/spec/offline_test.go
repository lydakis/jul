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
	cpOut1 := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "offline one", "--no-ci", "--no-review", "--json")
	var cp1 checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpOut1)).Decode(&cp1); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if cp1.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	traceOut1 := runCmd(t, repo, device.Env, julPath, "trace", "--prompt", "offline one", "--json")
	if !strings.Contains(traceOut1, "trace_sha") {
		t.Fatalf("expected trace output, got %s", traceOut1)
	}

	writeFile(t, repo, "offline.txt", "one\ntwo\n")
	cpOut2 := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "offline two", "--no-ci", "--no-review", "--json")
	var cp2 checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpOut2)).Decode(&cp2); err != nil {
		t.Fatalf("failed to decode second checkpoint output: %v", err)
	}
	if cp2.CheckpointSHA == "" {
		t.Fatalf("expected second checkpoint sha")
	}

	traceOut2 := runCmd(t, repo, device.Env, julPath, "trace", "--prompt", "offline two", "--json")
	if !strings.Contains(traceOut2, "trace_sha") {
		t.Fatalf("expected trace output, got %s", traceOut2)
	}

	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, repo, device.Env, julPath, "remote", "set", "origin")
	runCmd(t, repo, device.Env, julPath, "doctor")
	runCmd(t, repo, device.Env, julPath, "sync", "--json")

	// Verify keep refs and notes are pushed.
	keepRefs := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "for-each-ref", "--format=%(refname)", "refs/jul/keep")
	keepList := strings.Fields(strings.TrimSpace(keepRefs))
	if len(keepList) < 2 {
		t.Fatalf("expected keep refs on remote, got %s", keepRefs)
	}
	notes := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "for-each-ref", "--format=%(refname)", "refs/notes/jul")
	if strings.TrimSpace(notes) == "" {
		t.Fatalf("expected notes refs on remote")
	}
}
