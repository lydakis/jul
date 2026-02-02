//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIT_UNSUPPORTED_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)

	sub := t.TempDir()
	initRepo(t, sub, true)
	writeFile(t, sub, "sub.txt", "sub\n")
	runCmd(t, sub, nil, "git", "add", "sub.txt")
	runCmd(t, sub, nil, "git", "commit", "-m", "sub")

	runCmd(t, repo, nil, "git", "-c", "protocol.file.allow=always", "submodule", "add", sub, "submod")
	runCmd(t, repo, nil, "git", "commit", "-m", "add submodule")

	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")
	runCmd(t, repo, device.Env, julPath, "init", "demo")

	out, _ := runCmdAllowFailure(t, repo, device.Env, julPath, "sync")
	if !strings.Contains(strings.ToLower(out), "submodule") {
		t.Fatalf("expected submodule warning, got %s", out)
	}

	jsonOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var res syncResult
	if err := json.NewDecoder(strings.NewReader(jsonOut)).Decode(&res); err != nil {
		t.Fatalf("expected sync json output, got %v (%s)", err, jsonOut)
	}
	if res.DraftSHA == "" {
		t.Fatalf("expected draft sha after sync")
	}
}
