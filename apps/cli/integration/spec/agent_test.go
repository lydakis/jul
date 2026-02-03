//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/output"
)

func TestIT_AGENT_006(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/ci.toml", "[commands]\nfail = \"false\"\n")
	writeFile(t, repo, ".jul/policy.toml", "[promote.main]\nrequired_checks = [\"test\"]\n")
	writeFile(t, repo, "README.md", "fail ci\n")

	cpOut, err := runCmdAllowFailure(t, repo, device.Env, julPath, "checkpoint", "-m", "fail ci", "--no-review", "--json")
	if err == nil {
		t.Fatalf("expected checkpoint to fail")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Fatalf("expected checkpoint exit code 1, got %d", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("expected exit error for checkpoint, got %v", err)
	}
	cpRes := decodeErrorJSON(t, cpOut)
	if cpRes.Code == "" || cpRes.Message == "" {
		t.Fatalf("expected checkpoint error code/message, got %+v", cpRes)
	}
	if cpRes.Code != "checkpoint_ci_failed" {
		t.Fatalf("expected checkpoint_ci_failed, got %+v", cpRes)
	}
	if len(cpRes.NextActions) == 0 {
		t.Fatalf("expected checkpoint next_actions, got %+v", cpRes)
	}

	promoteOut, err := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main", "--json")
	if err == nil {
		t.Fatalf("expected promote to fail")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Fatalf("expected promote exit code 1, got %d", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("expected exit error for promote, got %v", err)
	}
	promoteRes := decodeErrorJSON(t, promoteOut)
	if promoteRes.Code == "" || promoteRes.Message == "" {
		t.Fatalf("expected promote error code/message, got %+v", promoteRes)
	}
	if promoteRes.Code != "promote_policy_failed" {
		t.Fatalf("expected promote_policy_failed, got %+v", promoteRes)
	}
	if len(promoteRes.NextActions) == 0 {
		t.Fatalf("expected promote next_actions, got %+v", promoteRes)
	}

	// Simulate network failure by pointing remote to an invalid path.
	runCmd(t, repo, nil, "git", "remote", "add", "origin", "/no/such/remote.git")

	syncOut, err := runCmdAllowFailure(t, repo, device.Env, julPath, "sync", "--json")
	if err == nil {
		t.Fatalf("expected sync to fail")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Fatalf("expected sync exit code 1, got %d", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("expected exit error for sync, got %v", err)
	}
	syncRes := decodeErrorJSON(t, syncOut)
	if syncRes.Code == "" || syncRes.Message == "" {
		t.Fatalf("expected sync error code/message, got %+v", syncRes)
	}
	if syncRes.Code != "sync_failed" {
		t.Fatalf("expected sync_failed, got %+v", syncRes)
	}
	if len(syncRes.NextActions) == 0 {
		t.Fatalf("expected sync next_actions, got %+v", syncRes)
	}
}

func decodeErrorJSON(t *testing.T, out string) output.ErrorOutput {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var res output.ErrorOutput
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("expected json output, got %v (%s)", err, out)
	}
	if err := dec.Decode(&struct{}{}); err == nil {
		t.Fatalf("expected only json output, got trailing data (%s)", out)
	} else if !errors.Is(err, io.EOF) {
		t.Fatalf("expected only json output, got trailing data (%s)", out)
	}
	return res
}
