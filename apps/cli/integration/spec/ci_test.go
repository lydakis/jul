//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIT_CI_002(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
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

	note := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", res.CheckpointSHA)
	if !strings.Contains(note, "status") {
		t.Fatalf("expected attestation note, got %s", note)
	}
}

func TestIT_CI_005(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\npass = \"true\"\n")
	writeFile(t, repo, "README.md", "coverage\n")

	out := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "ci: coverage", "--no-review", "--json")
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
	outPromote, err := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main")
	if err == nil {
		t.Fatalf("expected promote to be blocked for low coverage, got %s", outPromote)
	}
}
