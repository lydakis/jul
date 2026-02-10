//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lydakis/jul/cli/internal/notes"
	"github.com/lydakis/jul/cli/internal/output"
)

func TestIT_CI_002(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, repo, nil, "git", "push", "-u", "origin", "main")
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\npass = \"true\"\n")
	writeFile(t, repo, "README.md", "ci\n")

	out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "ci: ok", "--no-review", "--json")
	var res checkpointResult
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&res); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if res.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	runPath := waitForCIRunFile(t, filepath.Join(repo, ".jul", "ci", "runs"), 5*time.Second)
	run := waitForCIRunResult(t, runPath, 10*time.Second)
	if strings.TrimSpace(run.CommitSHA) != strings.TrimSpace(res.CheckpointSHA) {
		t.Fatalf("expected ci run commit %s, got %s", res.CheckpointSHA, run.CommitSHA)
	}
	if strings.TrimSpace(run.Status) == "" {
		t.Fatalf("expected ci run status, got %+v", run)
	}

	note := strings.TrimSpace(runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", res.CheckpointSHA))
	if len(note) > notes.MaxNoteSize {
		t.Fatalf("expected attestation note within size limit, got %d bytes", len(note))
	}
	var att map[string]any
	if err := json.Unmarshal([]byte(note), &att); err != nil {
		t.Fatalf("expected attestation note JSON, got %v (%s)", err, note)
	}
	if status, ok := att["status"]; !ok || strings.TrimSpace(fmt.Sprint(status)) == "" {
		t.Fatalf("expected attestation status, got %+v", att)
	}

	runCmd(t, repo, device.Env, julPath, "sync", "--json")

	remoteNote := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", res.CheckpointSHA)
	if !strings.Contains(remoteNote, "status") {
		t.Fatalf("expected attestation note to sync, got %s", remoteNote)
	}
}

func TestIT_CI_005(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\npass = \"true\"\n")
	writeFile(t, repo, ".jul/policy.toml", "[promote.main]\nmin_coverage_pct = 80\n")
	writeFile(t, repo, "README.md", "coverage\n")

	out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "ci: coverage", "--no-ci", "--no-review", "--json")
	var res checkpointResult
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&res); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if res.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	// Record low coverage attestation.
	runCmd(t, repo, device.Env, julPath, "ci", "run", "--coverage-line", "10", "--coverage-branch", "10", "--target", res.CheckpointSHA)

	// Expect promote to be blocked due to coverage policy.
	outPromote, err := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main", "--json")
	if err == nil {
		t.Fatalf("expected promote to be blocked for low coverage, got %s", outPromote)
	}
	var promoteErr output.ErrorOutput
	if err := json.NewDecoder(strings.NewReader(outPromote)).Decode(&promoteErr); err != nil {
		t.Fatalf("expected promote error json, got %v (%s)", err, outPromote)
	}
	if promoteErr.Code == "" || !strings.Contains(strings.ToLower(promoteErr.Message), "coverage") {
		t.Fatalf("expected coverage error, got %+v", promoteErr)
	}
	if strings.Count(promoteErr.Message, "%") < 2 {
		t.Fatalf("expected coverage message to include actual and threshold, got %s", promoteErr.Message)
	}
	foundBypass := false
	for _, action := range promoteErr.NextActions {
		if strings.TrimSpace(action.Command) == "jul promote --no-policy --json" {
			foundBypass = true
			break
		}
	}
	if !foundBypass {
		t.Fatalf("expected bypass next_action, got %+v", promoteErr.NextActions)
	}

	outBypass := runCmd(t, repo, device.Env, julPath, "promote", "--to", "main", "--no-policy", "--json")
	if !strings.Contains(outBypass, "\"status\"") {
		t.Fatalf("expected promote to succeed with --no-policy, got %s", outBypass)
	}
}

func TestIT_CI_003(t *testing.T) {
	root := t.TempDir()
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})

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
	writeFile(t, repoA, ".jul/ci.toml", "[commands]\npass = \"true\"\n")
	writeFile(t, repoB, ".jul/ci.toml", "[commands]\npass = \"true\"\n")
	writeFile(t, repoA, ".jul/policy.toml", "[promote.main]\nrequired_checks = [\"ci\"]\n")
	writeFile(t, repoB, ".jul/policy.toml", "[promote.main]\nrequired_checks = [\"ci\"]\n")

	writeFile(t, repoA, "README.md", "device a checkpoint\n")
	cpOut := runCmd(t, repoA, deviceA.Env, julPath, "checkpoint", "-m", "ci trust setup", "--no-ci", "--no-review", "--json")
	var cp checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpOut)).Decode(&cp); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if strings.TrimSpace(cp.CheckpointSHA) == "" {
		t.Fatalf("expected checkpoint sha from device A, got %+v", cp)
	}

	runCmd(t, repoA, deviceA.Env, julPath, "ci", "run", "--cmd", "true", "--target", cp.CheckpointSHA, "--json")
	runCmd(t, repoA, deviceA.Env, julPath, "sync", "--json")
	remoteNote := runCmd(t, repoA, nil, "git", "--git-dir", remoteDir, "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", cp.CheckpointSHA)
	if !strings.Contains(remoteNote, "\"status\"") {
		t.Fatalf("expected checkpoint attestation note on remote, got %s", remoteNote)
	}

	runCmd(t, repoB, deviceB.Env, julPath, "sync", "--json")
	runCmd(t, repoB, nil, "git", "fetch", "origin", "+refs/notes/jul/attestations/checkpoint:refs/notes/jul/attestations/checkpoint")
	note := runCmd(t, repoB, nil, "git", "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", cp.CheckpointSHA)
	if !strings.Contains(note, "\"status\"") {
		t.Fatalf("expected synced checkpoint attestation note on device B, got %s", note)
	}

	promoteOut, err := runCmdAllowFailure(t, repoB, deviceB.Env, julPath, "promote", "--to", "main", cp.CheckpointSHA, "--json")
	if err == nil {
		t.Fatalf("expected promote to reject remote-only attestation, got %s", promoteOut)
	}
	var promoteErr output.ErrorOutput
	if err := json.NewDecoder(strings.NewReader(promoteOut)).Decode(&promoteErr); err != nil {
		t.Fatalf("expected promote error json, got %v (%s)", err, promoteOut)
	}
	lower := strings.ToLower(promoteErr.Message)
	if !strings.Contains(lower, "local") && !strings.Contains(lower, "rerun") {
		t.Fatalf("expected promote to require local checks, got %+v", promoteErr)
	}
	foundRerun := false
	for _, action := range promoteErr.NextActions {
		if strings.Contains(action.Command, "jul ci run --target "+cp.CheckpointSHA) {
			foundRerun = true
			break
		}
	}
	if !foundRerun {
		t.Fatalf("expected rerun next_action for local CI, got %+v", promoteErr.NextActions)
	}
}

func TestIT_CI_004(t *testing.T) {
	root := t.TempDir()
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})

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
	writeFile(t, repoB, ".jul/ci.toml", "[commands]\npass = \"true\"\n")
	writeFile(t, repoB, ".jul/policy.toml", "[promote.main]\nrequired_checks = [\"ci\"]\n")

	writeFile(t, repoB, "b.txt", "from B\n")
	cpBOut := runCmd(t, repoB, deviceB.Env, julPath, "checkpoint", "-m", "feat: B", "--no-ci", "--no-review", "--json")
	var cpB checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpBOut)).Decode(&cpB); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if strings.TrimSpace(cpB.CheckpointSHA) == "" {
		t.Fatalf("expected checkpoint sha from device B, got %+v", cpB)
	}
	runCmd(t, repoB, deviceB.Env, julPath, "ci", "run", "--cmd", "true", "--target", cpB.CheckpointSHA, "--json")

	runCmd(t, repoA, deviceA.Env, julPath, "sync", "--json")
	writeFile(t, repoA, "a.txt", "from A\n")
	runCmd(t, repoA, deviceA.Env, julPath, "checkpoint", "-m", "feat: A", "--no-ci", "--no-review", "--json")

	syncOut := runCmd(t, repoB, deviceB.Env, julPath, "sync", "--json")
	var syncRes syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&syncRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if syncRes.Diverged || syncRes.BaseAdvanced {
		t.Fatalf("expected autorestack to resolve base advance, got %+v", syncRes)
	}
	if !syncRes.WorkspaceUpdated {
		t.Fatalf("expected workspace update from autorestack")
	}

	statusOut := runCmd(t, repoB, deviceB.Env, julPath, "status", "--json")
	var status output.Status
	if err := json.NewDecoder(strings.NewReader(statusOut)).Decode(&status); err != nil {
		t.Fatalf("failed to decode status output: %v (%s)", err, statusOut)
	}
	if status.LastCheckpoint == nil || strings.TrimSpace(status.LastCheckpoint.CommitSHA) == "" {
		t.Fatalf("expected latest checkpoint after restack, got %+v", status)
	}
	if !status.AttestationStale {
		t.Fatalf("expected inherited attestation to be stale after restack, got %+v", status)
	}
	if strings.TrimSpace(status.AttestationInheritedFrom) == "" {
		t.Fatalf("expected inherited attestation source, got %+v", status)
	}
	if strings.ToLower(strings.TrimSpace(status.AttestationStatus)) != "pass" {
		t.Fatalf("expected inherited pass status to be visible, got %+v", status)
	}

	restackedSHA := strings.TrimSpace(status.LastCheckpoint.CommitSHA)
	promoteOut, err := runCmdAllowFailure(t, repoB, deviceB.Env, julPath, "promote", "--to", "main", restackedSHA, "--json")
	if err == nil {
		t.Fatalf("expected promote to block stale inherited attestation, got %s", promoteOut)
	}
	var promoteErr output.ErrorOutput
	if err := json.NewDecoder(strings.NewReader(promoteOut)).Decode(&promoteErr); err != nil {
		t.Fatalf("expected promote error json, got %v (%s)", err, promoteOut)
	}
	if !strings.Contains(strings.ToLower(promoteErr.Message), "stale") {
		t.Fatalf("expected stale attestation failure, got %+v", promoteErr)
	}
	foundRerun := false
	for _, action := range promoteErr.NextActions {
		if strings.Contains(action.Command, "jul ci run --target "+restackedSHA) {
			foundRerun = true
			break
		}
	}
	if !foundRerun {
		t.Fatalf("expected rerun next_action for restacked checkpoint %s, got %+v", restackedSHA, promoteErr.NextActions)
	}
}
