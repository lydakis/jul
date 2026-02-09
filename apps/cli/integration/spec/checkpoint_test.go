//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lydakis/jul/cli/internal/output"
)

type checkpointResult struct {
	CheckpointSHA string `json:"CheckpointSHA"`
	DraftSHA      string `json:"DraftSHA"`
	ChangeID      string `json:"ChangeID"`
	TraceHead     string `json:"TraceHead"`
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

	// Second checkpoint should keep the same Change-Id and preserve anchor ref.
	writeFile(t, repo, "README.md", "checkpoint\nsecond\n")
	out2 := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: cp2", "--no-ci", "--no-review", "--json")
	var res2 checkpointResult
	if err := json.NewDecoder(strings.NewReader(out2)).Decode(&res2); err != nil {
		t.Fatalf("failed to decode second checkpoint output: %v", err)
	}
	if res2.CheckpointSHA == "" {
		t.Fatalf("expected second checkpoint sha")
	}
	if res2.ChangeID != res.ChangeID {
		t.Fatalf("expected Change-Id to remain %s, got %s", res.ChangeID, res2.ChangeID)
	}
	anchorTip2 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", anchorRef))
	if anchorTip2 != res.CheckpointSHA {
		t.Fatalf("expected anchor ref to remain %s, got %s", res.CheckpointSHA, anchorTip2)
	}
	workspaceTip2 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", workspaceRef))
	if workspaceTip2 != res2.CheckpointSHA {
		t.Fatalf("expected workspace ref to advance to %s, got %s", res2.CheckpointSHA, workspaceTip2)
	}
	changeTip2 := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", changeRef))
	if changeTip2 != res2.CheckpointSHA {
		t.Fatalf("expected change ref to advance to %s, got %s", res2.CheckpointSHA, changeTip2)
	}
}

func TestIT_CP_002(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\nfail = \"sleep 2; false\"\n")
	writeFile(t, repo, "README.md", "checkpoint with failing checks\n")

	out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: failing checks", "--no-review", "--json")
	var res checkpointResult
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&res); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if strings.TrimSpace(res.CheckpointSHA) == "" {
		t.Fatalf("expected checkpoint sha, got %+v", res)
	}
	if strings.TrimSpace(res.KeepRef) == "" {
		t.Fatalf("expected keep ref in checkpoint output, got %+v", res)
	}
	_ = runCmd(t, repo, nil, "git", "show-ref", res.KeepRef)

	runPath := waitForCIRunFile(t, filepath.Join(repo, ".jul", "ci", "runs"), 5*time.Second)
	initial := waitForCIRunStatus(t, runPath, 2*time.Second)
	if initial.Status != "running" {
		t.Fatalf("expected checkpoint to return before CI completion, got status %q", initial.Status)
	}

	run := waitForCIRunResult(t, runPath, 10*time.Second)
	if run.Status != "fail" {
		t.Fatalf("expected failing ci run status, got %+v", run)
	}
	if strings.TrimSpace(run.CommitSHA) != strings.TrimSpace(res.CheckpointSHA) {
		t.Fatalf("expected ci run commit %s, got %s", res.CheckpointSHA, run.CommitSHA)
	}

	note := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", res.CheckpointSHA)
	if !strings.Contains(note, `"status":"fail"`) {
		t.Fatalf("expected failing checkpoint attestation note, got %s", note)
	}
}

func TestIT_CP_002_Watch(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\nfail = \"false\"\n")
	writeFile(t, repo, "README.md", "checkpoint with failing checks watch\n")

	out, err := runCmdAllowFailure(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: failing checks watch", "--no-review", "--watch", "--json")
	if err == nil {
		t.Fatalf("expected checkpoint --watch to exit non-zero on failing checks, got %s", out)
	}

	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("expected checkpoint --watch output to contain json payload, got %s", out)
	}

	var res checkpointResult
	if err := json.NewDecoder(strings.NewReader(out[jsonStart:])).Decode(&res); err != nil {
		t.Fatalf("failed to decode checkpoint --watch output: %v (%s)", err, out)
	}
	if strings.TrimSpace(res.CheckpointSHA) == "" {
		t.Fatalf("expected checkpoint sha, got %+v", res)
	}
	if strings.TrimSpace(res.KeepRef) == "" {
		t.Fatalf("expected keep ref in checkpoint output, got %+v", res)
	}
	_ = runCmd(t, repo, nil, "git", "show-ref", res.KeepRef)

	note := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", res.CheckpointSHA)
	if !strings.Contains(note, `"status":"fail"`) {
		t.Fatalf("expected failing checkpoint attestation note, got %s", note)
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
	deviceA.Env["JUL_NO_SYNC"] = "1"
	deviceB.Env["JUL_NO_SYNC"] = "1"

	runCmd(t, repoA, deviceA.Env, julPath, "init", "demo")
	runCmd(t, repoB, deviceB.Env, julPath, "init", "demo")
	writeFile(t, repoB, ".jul/config.toml", "[sync]\nautorestack = false\n")

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
	keepList := strings.Fields(strings.TrimSpace(keepRefs))
	lastRef := keepList[len(keepList)-1]
	parts := strings.Split(lastRef, "/")
	checkpointSHA := parts[len(parts)-1]
	_ = runCmd(t, repoB, nil, "git", "rev-parse", checkpointSHA)

	syncOut := runCmd(t, repoB, deviceB.Env, julPath, "sync", "--json")
	var syncRes struct {
		Diverged      bool   `json:"Diverged"`
		RemoteProblem string `json:"RemoteProblem"`
	}
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&syncRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if !syncRes.Diverged && !strings.Contains(syncRes.RemoteProblem, "workspace lease") {
		t.Fatalf("expected workspace to be marked diverged, got %+v", syncRes)
	}

	promoteOut, err := runCmdAllowFailure(t, repoB, deviceB.Env, julPath, "promote", "--to", "main", "--json")
	if err == nil {
		t.Fatalf("expected promote to be blocked after diverged checkpoint, got %s", promoteOut)
	}
	var promoteErr output.ErrorOutput
	if err := json.NewDecoder(strings.NewReader(promoteOut)).Decode(&promoteErr); err != nil {
		t.Fatalf("expected promote error json, got %v (%s)", err, promoteOut)
	}
	if !strings.Contains(strings.ToLower(promoteErr.Message), "checkout") && !strings.Contains(strings.ToLower(promoteErr.Message), "restack") {
		t.Fatalf("expected promote to suggest checkout/restack, got %+v", promoteErr)
	}
}

func TestIT_CP_004(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")
	device.Env["JUL_NO_SYNC"] = "1"

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, "README.md", "trace base\n")
	traceOut := runCmd(t, repo, device.Env, julPath, "trace", "--prompt", "seed trace", "--json")
	var traceRes struct {
		TraceSHA string `json:"trace_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(traceOut)).Decode(&traceRes); err != nil {
		t.Fatalf("failed to decode trace output: %v", err)
	}
	if strings.TrimSpace(traceRes.TraceSHA) == "" {
		t.Fatalf("expected trace sha from explicit trace, got %s", traceOut)
	}

	writeFile(t, repo, "README.md", "trace flush candidate\n")
	cpOut := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: flush trace", "--no-ci", "--no-review", "--json")
	var cp checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpOut)).Decode(&cp); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if strings.TrimSpace(cp.CheckpointSHA) == "" {
		t.Fatalf("expected checkpoint sha, got %+v", cp)
	}
	if strings.TrimSpace(cp.TraceHead) == "" {
		t.Fatalf("expected trace head in checkpoint output, got %+v", cp)
	}

	checkpointTree := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", cp.CheckpointSHA+"^{tree}"))
	traceTree := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", cp.TraceHead+"^{tree}"))
	if checkpointTree != traceTree {
		t.Fatalf("expected checkpoint tree %s to match trace tree %s", checkpointTree, traceTree)
	}

	message := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", cp.CheckpointSHA)
	if !strings.Contains(message, "Trace-Head: "+cp.TraceHead) {
		t.Fatalf("expected Trace-Head trailer %s in checkpoint message: %s", cp.TraceHead, message)
	}
}

func TestIT_CP_005(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")
	device.Env["JUL_NO_SYNC"] = "1"

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	writeFile(t, repo, "README.md", "change-id lifecycle one\n")
	cp1Out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: one", "--no-ci", "--no-review", "--json")
	var cp1 checkpointResult
	if err := json.NewDecoder(strings.NewReader(cp1Out)).Decode(&cp1); err != nil {
		t.Fatalf("failed to decode first checkpoint output: %v", err)
	}
	if strings.TrimSpace(cp1.ChangeID) == "" {
		t.Fatalf("expected first checkpoint change id, got %+v", cp1)
	}

	writeFile(t, repo, "README.md", "change-id lifecycle two\n")
	cp2Out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: two", "--no-ci", "--no-review", "--json")
	var cp2 checkpointResult
	if err := json.NewDecoder(strings.NewReader(cp2Out)).Decode(&cp2); err != nil {
		t.Fatalf("failed to decode second checkpoint output: %v", err)
	}
	if strings.TrimSpace(cp2.ChangeID) == "" {
		t.Fatalf("expected second checkpoint change id, got %+v", cp2)
	}
	if cp1.ChangeID != cp2.ChangeID {
		t.Fatalf("expected pre-promote checkpoints to share change id %s, got %s", cp1.ChangeID, cp2.ChangeID)
	}

	promoteOut := runCmd(t, repo, device.Env, julPath, "promote", "--to", "main", "--no-policy", "--json")
	var promote struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(strings.NewReader(promoteOut)).Decode(&promote); err != nil {
		t.Fatalf("failed to decode promote output: %v", err)
	}
	if strings.TrimSpace(promote.Status) != "ok" {
		t.Fatalf("expected promote status ok, got %s (%s)", promote.Status, promoteOut)
	}

	writeFile(t, repo, "README.md", "change-id lifecycle three\n")
	cp3Out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: three", "--no-ci", "--no-review", "--json")
	var cp3 checkpointResult
	if err := json.NewDecoder(strings.NewReader(cp3Out)).Decode(&cp3); err != nil {
		t.Fatalf("failed to decode third checkpoint output: %v", err)
	}
	if strings.TrimSpace(cp3.ChangeID) == "" {
		t.Fatalf("expected post-promote checkpoint change id, got %+v", cp3)
	}
	if cp3.ChangeID == cp2.ChangeID {
		t.Fatalf("expected new change id after promote, got reused %s", cp3.ChangeID)
	}
}

type ciRunRecord struct {
	CommitSHA string `json:"commit_sha"`
	Status    string `json:"status"`
}

func waitForCIRunFile(t *testing.T, dir string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				return filepath.Join(dir, entry.Name())
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for ci run in %s", dir)
	return ""
}

func waitForCIRunResult(t *testing.T, runPath string, timeout time.Duration) ciRunRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastDecodeErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(runPath)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var run ciRunRecord
		if err := json.Unmarshal(data, &run); err != nil {
			lastDecodeErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if strings.TrimSpace(run.Status) != "" && strings.TrimSpace(run.Status) != "running" {
			return run
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastDecodeErr != nil {
		t.Fatalf("timed out waiting for ci run completion at %s (last decode error: %v)", runPath, lastDecodeErr)
	}
	t.Fatalf("timed out waiting for ci run completion at %s", runPath)
	return ciRunRecord{}
}

func waitForCIRunStatus(t *testing.T, runPath string, timeout time.Duration) ciRunRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastDecodeErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(runPath)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var run ciRunRecord
		if err := json.Unmarshal(data, &run); err != nil {
			lastDecodeErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if strings.TrimSpace(run.Status) != "" {
			return run
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastDecodeErr != nil {
		t.Fatalf("timed out waiting for ci run status at %s (last decode error: %v)", runPath, lastDecodeErr)
	}
	t.Fatalf("timed out waiting for ci run status at %s", runPath)
	return ciRunRecord{}
}
