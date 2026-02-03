package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type syncJSON struct {
	DraftSHA     string `json:"DraftSHA"`
	WorkspaceRef string `json:"WorkspaceRef"`
	SyncRef      string `json:"SyncRef"`
}

type checkpointJSON struct {
	CheckpointSHA string `json:"CheckpointSHA"`
	KeepRef       string `json:"KeepRef"`
}

func TestSmokeGitRemoteFlow(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "demo")
	remoteDir := filepath.Join(tmp, "remote.git")

	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	runCmd(t, tmp, nil, "git", "init", "--bare", remoteDir)
	runCmd(t, remoteDir, nil, "git", "symbolic-ref", "HEAD", "refs/heads/main")

	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	julPath := buildCLI(t)
	home := filepath.Join(tmp, "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo")

	writeFile(t, repo, "README.md", "hello\n")
	syncOut := runCmd(t, repo, env, julPath, "sync", "--json")
	var syncRes syncJSON
	decodeJSON(t, syncOut, &syncRes)
	if syncRes.SyncRef == "" || syncRes.WorkspaceRef == "" {
		t.Fatalf("expected sync refs, got %+v", syncRes)
	}
	runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "show-ref", syncRes.SyncRef)
	if _, err := runCmdAllowFailure(t, repo, nil, "git", "--git-dir", remoteDir, "show-ref", syncRes.WorkspaceRef); err != nil {
		// Workspace base may not exist until the first checkpoint in a new repo.
	}

	checkpointOut := runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review", "--json")
	var checkpointRes checkpointJSON
	decodeJSON(t, checkpointOut, &checkpointRes)
	if checkpointRes.KeepRef == "" || checkpointRes.CheckpointSHA == "" {
		t.Fatalf("expected keep ref and checkpoint sha")
	}
	runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "show-ref", checkpointRes.KeepRef)
}

func TestSmokeJulRemoteFlow(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "demo")
	reposDir := filepath.Join(tmp, "repos")
	remoteDir := filepath.Join(reposDir, "demo.git")

	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("failed to create repos dir: %v", err)
	}
	runCmd(t, reposDir, nil, "git", "init", "--bare", remoteDir)
	runCmd(t, remoteDir, nil, "git", "symbolic-ref", "HEAD", "refs/heads/main")

	julPath := buildCLI(t)
	home := filepath.Join(tmp, "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo", "--server", reposDir, "--create-remote")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, "README.md", "hello\n")
	syncOut := runCmd(t, repo, env, julPath, "sync", "--json")
	var syncRes syncJSON
	decodeJSON(t, syncOut, &syncRes)
	if syncRes.SyncRef == "" || syncRes.WorkspaceRef == "" {
		t.Fatalf("expected sync refs, got %+v", syncRes)
	}
	runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "show-ref", syncRes.SyncRef)
	if _, err := runCmdAllowFailure(t, repo, nil, "git", "--git-dir", remoteDir, "show-ref", syncRes.WorkspaceRef); err != nil {
		// Workspace base may not exist until the first checkpoint in a new repo.
	}

	checkpointOut := runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review", "--json")
	var checkpointRes checkpointJSON
	decodeJSON(t, checkpointOut, &checkpointRes)
	if checkpointRes.KeepRef == "" || checkpointRes.CheckpointSHA == "" {
		t.Fatalf("expected keep ref and checkpoint sha")
	}
	runCmd(t, repo, nil, "git", "--git-dir", remoteDir, "show-ref", checkpointRes.KeepRef)
}

func decodeJSON(t *testing.T, payload string, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(strings.NewReader(payload)).Decode(target); err != nil {
		t.Fatalf("failed to decode json output: %v", err)
	}
}
