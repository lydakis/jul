//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"errors"
	"io"
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

	cpOut, _ := runCmdAllowFailure(t, repo, device.Env, julPath, "checkpoint", "-m", "fail ci", "--no-review", "--json")
	cpRes := decodeErrorJSON(t, cpOut)
	if cpRes.Code == "" || cpRes.Message == "" {
		t.Fatalf("expected checkpoint error code/message, got %+v", cpRes)
	}

	promoteOut, _ := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main", "--json")
	promoteRes := decodeErrorJSON(t, promoteOut)
	if promoteRes.Code == "" || promoteRes.Message == "" {
		t.Fatalf("expected promote error code/message, got %+v", promoteRes)
	}

	// Simulate network failure by pointing remote to an invalid path.
	runCmd(t, repo, nil, "git", "remote", "add", "origin", "/no/such/remote.git")

	syncOut, _ := runCmdAllowFailure(t, repo, device.Env, julPath, "sync", "--json")
	syncRes := decodeErrorJSON(t, syncOut)
	if syncRes.Code == "" || syncRes.Message == "" {
		t.Fatalf("expected sync error code/message, got %+v", syncRes)
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
