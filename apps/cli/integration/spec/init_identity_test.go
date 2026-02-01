//go:build jul_integ_spec

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIT_INIT_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)

	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	out := runCmd(t, repo, device.Env, julPath, "init", "demo")
	if !strings.Contains(out, "No remote configured") && !strings.Contains(out, "local only") {
		t.Fatalf("expected local-only init message, got: %s", out)
	}

	runCmd(t, repo, device.Env, julPath, "status")
	id1 := readDeviceID(t, device.Home)
	if id1 == "" {
		t.Fatalf("expected device id to be created")
	}
	runCmd(t, repo, device.Env, julPath, "status")
	id2 := readDeviceID(t, device.Home)
	if id1 != id2 {
		t.Fatalf("expected stable device id, got %q vs %q", id1, id2)
	}

	assertJulIgnored(t, repo)

	head := strings.TrimSpace(runCmd(t, repo, nil, "git", "symbolic-ref", "HEAD"))
	if head != "refs/heads/jul/@" {
		t.Fatalf("expected HEAD to point to refs/heads/jul/@, got %s", head)
	}
}

func TestIT_DOCTOR_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteNoCustom})
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	runCmd(t, repo, device.Env, julPath, "remote", "set", "origin")

	out, err := runCmdAllowFailure(t, repo, device.Env, julPath, "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "checkpoint_sync: disabled") {
		t.Fatalf("expected checkpoint sync disabled, got: %s", out)
	}
	if !strings.Contains(out, "draft_sync: disabled") {
		t.Fatalf("expected draft sync disabled, got: %s", out)
	}

	writeFile(t, repo, "secret.txt", "data\n")
	syncOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	if !strings.Contains(syncOut, "DraftSHA") {
		t.Fatalf("expected sync json output, got %s", syncOut)
	}

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	ensureRemoteRefMissing(t, remoteDir, syncRef)
}

func TestIT_DOCTOR_002(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFFDraft})
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	out, err := runCmdAllowFailure(t, repo, device.Env, julPath, "doctor")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "checkpoint_sync: enabled") {
		t.Fatalf("expected checkpoint sync enabled, got: %s", out)
	}
	if !strings.Contains(out, "draft_sync: disabled") {
		t.Fatalf("expected draft sync disabled, got: %s", out)
	}

	writeFile(t, repo, "draft.txt", "change\n")
	syncOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	if !strings.Contains(syncOut, "DraftSHA") {
		t.Fatalf("expected sync json output, got %s", syncOut)
	}

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	ensureRemoteRefMissing(t, remoteDir, syncRef)
}

func assertJulIgnored(t *testing.T, repo string) {
	t.Helper()
	gitignore := filepath.Join(repo, ".gitignore")
	infoExclude := filepath.Join(repo, ".git", "info", "exclude")
	ignored := false
	for _, path := range []string{gitignore, infoExclude} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), ".jul") {
			ignored = true
			break
		}
	}
	if !ignored {
		status := runCmd(t, repo, nil, "git", "status", "--porcelain")
		if strings.Contains(status, ".jul") {
			t.Fatalf("expected .jul to be ignored by git, got status: %s", status)
		}
	}
}
