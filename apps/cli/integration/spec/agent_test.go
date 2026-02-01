//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIT_AGENT_006(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\nfail = \"false\"\n")
	writeFile(t, repo, "README.md", "fail ci\n")

	cpOut, _ := runCmdAllowFailure(t, repo, device.Env, julPath, "checkpoint", "-m", "fail ci", "--json")
	var cpRes map[string]any
	if err := json.NewDecoder(strings.NewReader(cpOut)).Decode(&cpRes); err != nil {
		t.Fatalf("expected checkpoint to emit json on error, got %v (%s)", err, cpOut)
	}

	promoteOut, _ := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main", "--json")
	var promoteRes map[string]any
	if err := json.NewDecoder(strings.NewReader(promoteOut)).Decode(&promoteRes); err != nil {
		t.Fatalf("expected promote to emit json error, got %v (%s)", err, promoteOut)
	}

	// Simulate network failure by pointing remote to an invalid path.
	runCmd(t, repo, nil, "git", "remote", "add", "origin", "/no/such/remote.git")

	syncOut, _ := runCmdAllowFailure(t, repo, device.Env, julPath, "sync", "--json")
	var syncRes map[string]any
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&syncRes); err != nil {
		t.Fatalf("expected sync to emit json error, got %v (%s)", err, syncOut)
	}
}
