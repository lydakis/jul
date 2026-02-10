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
