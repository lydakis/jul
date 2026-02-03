//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"sort"
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
	var trace1 struct {
		TraceSHA string `json:"trace_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(traceOut1)).Decode(&trace1); err != nil {
		t.Fatalf("failed to decode trace output: %v", err)
	}
	if trace1.TraceSHA == "" {
		t.Fatalf("expected trace sha, got %s", traceOut1)
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
	var trace2 struct {
		TraceSHA string `json:"trace_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(traceOut2)).Decode(&trace2); err != nil {
		t.Fatalf("failed to decode trace output: %v", err)
	}
	if trace2.TraceSHA == "" {
		t.Fatalf("expected trace sha, got %s", traceOut2)
	}

	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, repo, device.Env, julPath, "remote", "set", "origin")
	runCmd(t, repo, device.Env, julPath, "doctor")
	runCmd(t, repo, device.Env, julPath, "sync", "--json")

	workspaceRef := "refs/jul/workspaces/tester/@"
	localWorkspace := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", workspaceRef))
	remoteWorkspace := strings.TrimSpace(runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "rev-parse", workspaceRef))
	if localWorkspace != remoteWorkspace {
		t.Fatalf("expected remote workspace ref to match local, got %s vs %s", localWorkspace, remoteWorkspace)
	}

	changeRef := "refs/jul/changes/" + cp2.ChangeID
	localChange := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", changeRef))
	remoteChange := strings.TrimSpace(runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "rev-parse", changeRef))
	if localChange != remoteChange {
		t.Fatalf("expected remote change ref to match local, got %s vs %s", localChange, remoteChange)
	}

	anchorRef := "refs/jul/anchors/" + cp2.ChangeID
	localAnchor := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", anchorRef))
	remoteAnchor := strings.TrimSpace(runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "rev-parse", anchorRef))
	if localAnchor != remoteAnchor {
		t.Fatalf("expected remote anchor ref to match local, got %s vs %s", localAnchor, remoteAnchor)
	}
	if localAnchor != cp1.CheckpointSHA {
		t.Fatalf("expected anchor ref to stay on first checkpoint %s, got %s", cp1.CheckpointSHA, localAnchor)
	}

	// Verify keep refs and notes are pushed.
	localKeep := runCmd(t, repo, nil, "git", "for-each-ref", "--format=%(refname)", "refs/jul/keep")
	localKeepList := strings.Fields(strings.TrimSpace(localKeep))
	if len(localKeepList) < 2 {
		t.Fatalf("expected keep refs locally, got %s", localKeep)
	}
	for _, ref := range localKeepList {
		ensureRemoteRefExists(t, remoteDir, ref)
	}
	remoteKeep := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "for-each-ref", "--format=%(refname)", "refs/jul/keep")
	remoteKeepList := strings.Fields(strings.TrimSpace(remoteKeep))
	sort.Strings(localKeepList)
	sort.Strings(remoteKeepList)
	if strings.Join(localKeepList, ",") != strings.Join(remoteKeepList, ",") {
		t.Fatalf("expected remote keep refs to match local, got local=%v remote=%v", localKeepList, remoteKeepList)
	}

	localNotes := runCmd(t, repo, nil, "git", "for-each-ref", "--format=%(refname)", "refs/notes/jul")
	remoteNotes := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "for-each-ref", "--format=%(refname)", "refs/notes/jul")
	if strings.TrimSpace(remoteNotes) == "" {
		t.Fatalf("expected notes refs on remote")
	}
	localNotesList := strings.Fields(strings.TrimSpace(localNotes))
	remoteNotesList := strings.Fields(strings.TrimSpace(remoteNotes))
	sort.Strings(localNotesList)
	sort.Strings(remoteNotesList)
	if strings.Join(localNotesList, ",") != strings.Join(remoteNotesList, ",") {
		t.Fatalf("expected remote notes to match local, got local=%v remote=%v", localNotesList, remoteNotesList)
	}

	_ = runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "notes", "--ref", "refs/notes/jul/traces", "show", trace1.TraceSHA)
	_ = runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "notes", "--ref", "refs/notes/jul/traces", "show", trace2.TraceSHA)
}
