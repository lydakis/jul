//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestIT_PROMOTE_REBASE_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, "file.txt", "one\n")
	out1 := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: one", "--no-ci", "--no-review", "--json")
	var cp1 checkpointResult
	if err := json.NewDecoder(strings.NewReader(out1)).Decode(&cp1); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}
	writeFile(t, repo, "file.txt", "one\ntwo\n")
	out2 := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: two", "--no-ci", "--no-review", "--json")
	var cp2 checkpointResult
	if err := json.NewDecoder(strings.NewReader(out2)).Decode(&cp2); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}

	outPromote, err := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main", "--rebase")
	if err != nil {
		t.Fatalf("expected rebase promote to succeed, got %v\n%s", err, outPromote)
	}

	mainTip := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "refs/heads/main"))
	if mainTip != cp2.CheckpointSHA {
		t.Fatalf("expected main to advance to latest checkpoint")
	}
}

func TestIT_PROMOTE_REWRITE_001(t *testing.T) {
	root := t.TempDir()
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})

	seed := filepath.Join(root, "seed")
	initRepo(t, seed, true)
	runCmd(t, seed, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, seed, nil, "git", "push", "-u", "origin", "main")

	repo := filepath.Join(root, "repo")
	runCmd(t, root, nil, "git", "clone", remoteDir, repo)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")
	runCmd(t, repo, device.Env, julPath, "init", "demo")

	writeFile(t, repo, "file.txt", "change\n")
	runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "feat: promote", "--no-ci", "--no-review", "--json")

	// Rewrite remote main to a different commit.
	rewrite := filepath.Join(root, "rewrite")
	runCmd(t, root, nil, "git", "clone", remoteDir, rewrite)
	runCmd(t, rewrite, nil, "git", "config", "user.name", "Rewrite")
	runCmd(t, rewrite, nil, "git", "config", "user.email", "rewrite@example.com")
	writeFile(t, rewrite, "rewritten.txt", "rewrite\n")
	runCmd(t, rewrite, nil, "git", "add", "rewritten.txt")
	runCmd(t, rewrite, nil, "git", "commit", "-m", "rewrite")
	runCmd(t, rewrite, nil, "git", "push", "--force", "origin", "main")

	outPromote, err := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main")
	if err == nil {
		t.Fatalf("expected promote to refuse rewritten target, got %s", outPromote)
	}
	if !strings.Contains(outPromote, "fast-forward") {
		t.Fatalf("expected fast-forward warning, got %s", outPromote)
	}
}
