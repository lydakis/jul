//go:build jul_integ_spec

package integration

import (
	"encoding/json"
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

	writeFile(t, repo, "promote.txt", "publish\n")
	cpOut := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: publish", "--no-ci", "--no-review", "--json")
	var cpRes checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpOut)).Decode(&cpRes); err != nil {
		t.Fatalf("decode checkpoint output: %v", err)
	}
	if cpRes.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	promoteOut := runCmd(t, repo, device.Env, julPath, "promote", "--to", "main", "--json")
	var promoteRes struct {
		Status    string `json:"status"`
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(promoteOut)).Decode(&promoteRes); err != nil {
		t.Fatalf("decode promote output: %v", err)
	}
	if promoteRes.Status != "ok" {
		t.Fatalf("expected promote ok, got %s", promoteRes.Status)
	}

	remoteMain := strings.TrimSpace(runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "rev-parse", "refs/heads/main"))
	if promoteRes.CommitSHA != remoteMain {
		t.Fatalf("expected promote to advance remote main to %s, got %s", promoteRes.CommitSHA, remoteMain)
	}
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

	writeFile(t, repo, "checkpoint.txt", "ok\n")
	cpOut := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: ok", "--no-ci", "--no-review", "--json")
	var cpRes checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpOut)).Decode(&cpRes); err != nil {
		t.Fatalf("decode checkpoint output: %v", err)
	}
	if cpRes.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	runCmd(t, repo, device.Env, julPath, "sync", "--json")

	workspaceRef := "refs/jul/workspaces/tester/@"
	ensureRemoteRefExists(t, remoteDir, workspaceRef)
	if strings.TrimSpace(cpRes.KeepRef) != "" {
		ensureRemoteRefExists(t, remoteDir, cpRes.KeepRef)
	}
	notes := runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "for-each-ref", "--format=%(refname)", "refs/notes/jul")
	if strings.TrimSpace(notes) == "" {
		t.Fatalf("expected notes to sync, got empty refs")
	}
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
