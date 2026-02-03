//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIT_SEC_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	writeFile(t, repo, "secrets.txt", "AKIA1234567890ABCDEF\n")
	syncOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var res syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&res); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if res.DraftSHA == "" {
		t.Fatalf("expected draft sha")
	}
	if !strings.Contains(res.RemoteProblem, "draft sync blocked") {
		t.Fatalf("expected secret scan to block push, got %+v", res)
	}

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	ensureRemoteRefMissing(t, remoteDir, syncRef)

	allowOut := runCmd(t, repo, device.Env, julPath, "sync", "--allow-secrets", "--json")
	var allowRes syncResult
	if err := json.NewDecoder(strings.NewReader(allowOut)).Decode(&allowRes); err != nil {
		t.Fatalf("failed to decode allow-secrets sync output: %v", err)
	}
	if allowRes.DraftSHA == "" {
		t.Fatalf("expected draft sha on allow-secrets sync, got %s", allowOut)
	}
	ensureRemoteRefExists(t, remoteDir, syncRef)
}
