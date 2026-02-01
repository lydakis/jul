//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type checkpointResult struct {
	CheckpointSHA string `json:"CheckpointSHA"`
	DraftSHA      string `json:"DraftSHA"`
	ChangeID      string `json:"ChangeID"`
	KeepRef       string `json:"KeepRef"`
	WorkspaceRef  string `json:"WorkspaceRef"`
	SyncRef       string `json:"SyncRef"`
}

func TestIT_CP_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, "README.md", "checkpoint\n")

	out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: cp", "--no-ci", "--no-review", "--json")
	var res checkpointResult
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&res); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if res.CheckpointSHA == "" || res.ChangeID == "" {
		t.Fatalf("expected checkpoint sha and change id, got %+v", res)
	}

	msg := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", res.CheckpointSHA)
	if !strings.Contains(msg, "Change-Id:") {
		t.Fatalf("expected Change-Id trailer, got %s", msg)
	}

	workspaceRef := "refs/jul/workspaces/tester/@"
	workspaceTip := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", workspaceRef))
	if workspaceTip != res.CheckpointSHA {
		t.Fatalf("expected workspace ref to point at checkpoint")
	}

	changeRef := "refs/jul/changes/" + res.ChangeID
	changeTip := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", changeRef))
	if changeTip != res.CheckpointSHA {
		t.Fatalf("expected change ref to point at checkpoint")
	}

	if strings.TrimSpace(res.KeepRef) == "" {
		t.Fatalf("expected keep ref")
	}
	_ = runCmd(t, repo, nil, "git", "show-ref", res.KeepRef)

	anchorRef := "refs/jul/anchors/" + res.ChangeID
	anchorTip := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", anchorRef))
	if anchorTip != res.CheckpointSHA {
		t.Fatalf("expected anchor ref to point at checkpoint")
	}

	newDraftParent := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", res.DraftSHA+"^"))
	if newDraftParent != res.CheckpointSHA {
		t.Fatalf("expected new draft to be based on checkpoint")
	}
}

func TestIT_CP_003(t *testing.T) {
	root := t.TempDir()
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteSelective, FFOnlyPrefixes: []string{"refs/jul/workspaces/"}})

	seed := filepath.Join(root, "seed")
	initRepo(t, seed, true)
	runCmd(t, seed, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, seed, nil, "git", "push", "-u", "origin", "main")

	repoA := filepath.Join(root, "repoA")
	repoB := filepath.Join(root, "repoB")
	runCmd(t, root, nil, "git", "clone", remoteDir, repoA)
	runCmd(t, root, nil, "git", "clone", remoteDir, repoB)

	julPath := buildCLI(t)
	deviceA := newDeviceEnv(t, "devA")
	deviceB := newDeviceEnv(t, "devB")

	runCmd(t, repoA, deviceA.Env, julPath, "init", "demo")
	runCmd(t, repoB, deviceB.Env, julPath, "init", "demo")

	writeFile(t, repoA, "file.txt", "A\n")
	outA := runCmd(t, repoA, deviceA.Env, julPath, "checkpoint", "-m", "feat: A", "--no-ci", "--no-review", "--json")
	var resA checkpointResult
	if err := json.NewDecoder(strings.NewReader(outA)).Decode(&resA); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if resA.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	writeFile(t, repoB, "file.txt", "B\n")
	outB, err := runCmdAllowFailure(t, repoB, deviceB.Env, julPath, "checkpoint", "-m", "feat: B", "--no-ci", "--no-review", "--json")
	if err == nil {
		t.Fatalf("expected checkpoint to fail due to non-ff workspace push, got %s", outB)
	}

	keepRefs := runCmd(t, repoB, nil, "git", "for-each-ref", "--format=%(refname)", "refs/jul/keep/tester/@")
	if strings.TrimSpace(keepRefs) == "" {
		t.Fatalf("expected keep refs to exist after failed checkpoint")
	}
}
