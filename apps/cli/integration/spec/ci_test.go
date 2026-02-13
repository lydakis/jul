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

func TestIT_CI_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/config.toml", "[ci]\nrun_on_draft = true\ndraft_ci_blocking = false\n")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\nslow = \"sleep 3; true\"\n")
	writeFile(t, repo, "README.md", "draft ci coalesce v1\n")

	syncOut1 := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var first syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut1)).Decode(&first); err != nil {
		t.Fatalf("failed to decode first sync output: %v (%s)", err, syncOut1)
	}
	if strings.TrimSpace(first.DraftSHA) == "" {
		t.Fatalf("expected first draft sha, got %+v", first)
	}

	waitForCIStatusMatch(t, repo, device.Env, julPath, 4*time.Second, func(status output.CIStatusJSON) bool {
		return strings.TrimSpace(status.CI.Status) == "running" && strings.TrimSpace(status.CI.RunningSHA) == strings.TrimSpace(first.DraftSHA)
	})

	writeFile(t, repo, "README.md", "draft ci coalesce v2\n")
	syncOut2 := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var second syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut2)).Decode(&second); err != nil {
		t.Fatalf("failed to decode second sync output: %v (%s)", err, syncOut2)
	}
	if strings.TrimSpace(second.DraftSHA) == "" {
		t.Fatalf("expected second draft sha, got %+v", second)
	}
	if strings.TrimSpace(second.DraftSHA) == strings.TrimSpace(first.DraftSHA) {
		t.Fatalf("expected second sync to advance draft sha, got %s", second.DraftSHA)
	}

	status := waitForCIStatusMatch(t, repo, device.Env, julPath, 4*time.Second, func(status output.CIStatusJSON) bool {
		return strings.TrimSpace(status.CI.Status) == "running" && strings.TrimSpace(status.CI.RunningSHA) == strings.TrimSpace(second.DraftSHA)
	})
	if strings.TrimSpace(status.CI.CurrentDraftSHA) != strings.TrimSpace(second.DraftSHA) {
		t.Fatalf("expected ci status current draft %s, got %+v", second.DraftSHA, status.CI)
	}
	if strings.TrimSpace(status.CI.RunningSHA) != strings.TrimSpace(second.DraftSHA) {
		t.Fatalf("expected running sha %s, got %+v", second.DraftSHA, status.CI)
	}
	if status.CI.RunningPID <= 0 {
		t.Fatalf("expected running pid in ci status, got %+v", status.CI)
	}

	runs := waitForCIRunsMatch(t, repo, device.Env, julPath, 6*time.Second, func(runs output.CIRunsJSON) bool {
		if len(runs.Runs) != 2 {
			return false
		}
		firstIdx := -1
		secondIdx := -1
		runningCount := 0
		for i := range runs.Runs {
			run := runs.Runs[i]
			sha := strings.TrimSpace(run.CommitSHA)
			status := strings.TrimSpace(run.Status)
			if sha == strings.TrimSpace(first.DraftSHA) {
				firstIdx = i
			}
			if sha == strings.TrimSpace(second.DraftSHA) {
				secondIdx = i
			}
			if status == "running" {
				runningCount++
			}
		}
		if firstIdx == -1 || secondIdx == -1 || runningCount != 1 {
			return false
		}
		return strings.TrimSpace(runs.Runs[firstIdx].Status) == "canceled" &&
			strings.TrimSpace(runs.Runs[secondIdx].Status) == "running"
	})
	firstIdx := -1
	secondIdx := -1
	runningCount := 0
	for i := range runs.Runs {
		run := runs.Runs[i]
		sha := strings.TrimSpace(run.CommitSHA)
		if sha == strings.TrimSpace(first.DraftSHA) {
			firstIdx = i
		}
		if sha == strings.TrimSpace(second.DraftSHA) {
			secondIdx = i
		}
		if strings.TrimSpace(run.Status) == "running" {
			runningCount++
		}
	}
	if firstIdx == -1 || secondIdx == -1 {
		t.Fatalf("expected first/second run records, got %+v", runs.Runs)
	}
	if strings.TrimSpace(runs.Runs[firstIdx].Status) != "canceled" {
		t.Fatalf("expected stale run for %s to be canceled, got %+v", first.DraftSHA, runs.Runs[firstIdx])
	}
	if strings.TrimSpace(runs.Runs[secondIdx].Status) != "running" {
		t.Fatalf("expected latest run for %s to be running, got %+v", second.DraftSHA, runs.Runs[secondIdx])
	}
	if runningCount != 1 {
		t.Fatalf("expected exactly one running run, got %d (%+v)", runningCount, runs.Runs)
	}

	waitForCIStatusMatch(t, repo, device.Env, julPath, 8*time.Second, func(status output.CIStatusJSON) bool {
		return strings.TrimSpace(status.CI.CurrentDraftSHA) == strings.TrimSpace(second.DraftSHA) &&
			strings.TrimSpace(status.CI.CompletedSHA) == strings.TrimSpace(second.DraftSHA) &&
			strings.TrimSpace(status.CI.RunningSHA) == "" &&
			status.CI.ResultsCurrent &&
			strings.TrimSpace(status.CI.Status) == "pass"
	})

	writeFile(t, repo, "README.md", "draft ci coalesce v3\n")
	syncOut3 := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var third syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut3)).Decode(&third); err != nil {
		t.Fatalf("failed to decode third sync output: %v (%s)", err, syncOut3)
	}
	if strings.TrimSpace(third.DraftSHA) == "" {
		t.Fatalf("expected third draft sha, got %+v", third)
	}
	if strings.TrimSpace(third.DraftSHA) == strings.TrimSpace(second.DraftSHA) {
		t.Fatalf("expected third sync to advance draft sha, got %s", third.DraftSHA)
	}

	staleStatus := waitForCIStatusMatch(t, repo, device.Env, julPath, 6*time.Second, func(status output.CIStatusJSON) bool {
		return strings.TrimSpace(status.CI.Status) == "running" &&
			strings.TrimSpace(status.CI.CurrentDraftSHA) == strings.TrimSpace(third.DraftSHA) &&
			strings.TrimSpace(status.CI.RunningSHA) == strings.TrimSpace(third.DraftSHA) &&
			strings.TrimSpace(status.CI.CompletedSHA) == strings.TrimSpace(second.DraftSHA) &&
			!status.CI.ResultsCurrent
	})
	if len(staleStatus.CI.Results) == 0 {
		t.Fatalf("expected stale ci status to include previous completed results, got %+v", staleStatus.CI)
	}
	if staleStatus.CI.RunningPID <= 0 {
		t.Fatalf("expected stale ci status to include running pid, got %+v", staleStatus.CI)
	}

	runCmd(t, repo, device.Env, julPath, "ci", "run", "--cmd", "true", "--target", third.DraftSHA, "--json")
	manualStatus := waitForCIStatusMatch(t, repo, device.Env, julPath, 6*time.Second, func(status output.CIStatusJSON) bool {
		return strings.TrimSpace(status.CI.CurrentDraftSHA) == strings.TrimSpace(third.DraftSHA) &&
			strings.TrimSpace(status.CI.CompletedSHA) == strings.TrimSpace(third.DraftSHA) &&
			status.CI.ResultsCurrent
	})
	if strings.TrimSpace(manualStatus.CI.CompletedSHA) != strings.TrimSpace(third.DraftSHA) || !manualStatus.CI.ResultsCurrent {
		t.Fatalf("expected manual ci run targeting current draft to supersede prior draft result, got %+v", manualStatus.CI)
	}

	runCmd(t, repo, device.Env, julPath, "ci", "run", "--cmd", "true", "--target", first.DraftSHA, "--json")
	nonDraftManualStatus := waitForCIStatusMatch(t, repo, device.Env, julPath, 6*time.Second, func(status output.CIStatusJSON) bool {
		return strings.TrimSpace(status.CI.CurrentDraftSHA) == strings.TrimSpace(third.DraftSHA) &&
			strings.TrimSpace(status.CI.CompletedSHA) == strings.TrimSpace(third.DraftSHA) &&
			status.CI.ResultsCurrent
	})
	if strings.TrimSpace(nonDraftManualStatus.CI.CompletedSHA) != strings.TrimSpace(third.DraftSHA) || !nonDraftManualStatus.CI.ResultsCurrent {
		t.Fatalf("expected manual ci run targeting non-draft sha to keep draft ci status, got %+v", nonDraftManualStatus.CI)
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
	restackedSHA := strings.TrimSpace(status.LastCheckpoint.CommitSHA)
	if restackedSHA == strings.TrimSpace(cpB.CheckpointSHA) {
		t.Fatalf("expected restack to rewrite checkpoint sha; still %s", restackedSHA)
	}
	if !status.AttestationStale {
		t.Fatalf("expected inherited attestation to be stale after restack, got %+v", status)
	}
	if strings.TrimSpace(status.AttestationInheritedFrom) != strings.TrimSpace(cpB.CheckpointSHA) {
		t.Fatalf("expected inherited attestation source %s, got %+v", cpB.CheckpointSHA, status)
	}
	if strings.ToLower(strings.TrimSpace(status.AttestationStatus)) != "pass" {
		t.Fatalf("expected inherited pass status to be visible, got %+v", status)
	}

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

func waitForCIStatusMatch(t *testing.T, repo string, env map[string]string, julPath string, timeout time.Duration, match func(status output.CIStatusJSON) bool) output.CIStatusJSON {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last output.CIStatusJSON
	var lastErr error
	var lastOut string
	for time.Now().Before(deadline) {
		out, err := runCmdAllowFailure(t, repo, env, julPath, "ci", "status", "--json")
		if err != nil && strings.TrimSpace(out) == "" {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		lastOut = out
		var status output.CIStatusJSON
		if err := json.NewDecoder(strings.NewReader(lastOut)).Decode(&status); err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		last = status
		if match(status) {
			return status
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("timed out waiting for ci status match (last decode err: %v, output=%s)", lastErr, lastOut)
	}
	t.Fatalf("timed out waiting for ci status match; last status: %+v", last.CI)
	return output.CIStatusJSON{}
}

func waitForCIRunsMatch(t *testing.T, repo string, env map[string]string, julPath string, timeout time.Duration, match func(runs output.CIRunsJSON) bool) output.CIRunsJSON {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last output.CIRunsJSON
	var lastErr error
	var lastOut string
	for time.Now().Before(deadline) {
		out, err := runCmdAllowFailure(t, repo, env, julPath, "ci", "list", "--limit", "10", "--json")
		if err != nil && strings.TrimSpace(out) == "" {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		lastOut = out
		var runs output.CIRunsJSON
		if err := json.NewDecoder(strings.NewReader(lastOut)).Decode(&runs); err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		last = runs
		if match(runs) {
			return runs
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("timed out waiting for ci runs match (last decode err: %v, output=%s)", lastErr, lastOut)
	}
	t.Fatalf("timed out waiting for ci runs match; last runs: %+v", last.Runs)
	return output.CIRunsJSON{}
}
