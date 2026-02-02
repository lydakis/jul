//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/output"
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

	outPromote := runCmd(t, repo, device.Env, julPath, "promote", "--to", "main", "--rebase", "--json")
	var promoteRes struct {
		Status     string `json:"status"`
		BaseMarker string `json:"base_marker_sha"`
		CommitSHA  string `json:"commit_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(outPromote)).Decode(&promoteRes); err != nil {
		t.Fatalf("decode promote output: %v", err)
	}
	if promoteRes.Status != "ok" {
		t.Fatalf("expected promote ok, got %s", promoteRes.Status)
	}

	mainTip := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "refs/heads/main"))
	if mainTip != cp2.CheckpointSHA {
		t.Fatalf("expected main to advance to latest checkpoint")
	}
	if promoteRes.CommitSHA != mainTip {
		t.Fatalf("expected promote commit sha %s, got %s", mainTip, promoteRes.CommitSHA)
	}

	headSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", "HEAD"))
	headMsg := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", headSHA)
	if !strings.Contains(headMsg, "Jul-Type: workspace-base") {
		t.Fatalf("expected workspace base marker, got %s", headMsg)
	}
	headParent := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", headSHA+"^"))
	if headParent != cp2.CheckpointSHA {
		t.Fatalf("expected base marker parent %s, got %s", cp2.CheckpointSHA, headParent)
	}

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	draftSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", syncRef))
	draftParent := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", draftSHA+"^"))
	if draftParent != headSHA {
		t.Fatalf("expected new draft parent %s, got %s", headSHA, draftParent)
	}

	anchorRef := "refs/jul/anchors/" + cp2.ChangeID
	anchorSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", anchorRef))
	metaNote := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/meta", "show", anchorSHA)
	if !strings.Contains(metaNote, "promote_events") {
		t.Fatalf("expected promote metadata note, got %s", metaNote)
	}

	reverseNote := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/change-id", "show", cp2.CheckpointSHA)
	if !strings.Contains(reverseNote, cp2.ChangeID) {
		t.Fatalf("expected reverse index note, got %s", reverseNote)
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

	// Rewrite remote main to a different root commit.
	rewrite := filepath.Join(root, "rewrite")
	runCmd(t, root, nil, "git", "clone", remoteDir, rewrite)
	runCmd(t, rewrite, nil, "git", "config", "user.name", "Rewrite")
	runCmd(t, rewrite, nil, "git", "config", "user.email", "rewrite@example.com")
	runCmd(t, rewrite, nil, "git", "checkout", "--orphan", "rewrite")
	runCmd(t, rewrite, nil, "git", "rm", "-rf", ".")
	writeFile(t, rewrite, "rewritten.txt", "rewrite\n")
	runCmd(t, rewrite, nil, "git", "add", "rewritten.txt")
	runCmd(t, rewrite, nil, "git", "commit", "-m", "rewrite")
	runCmd(t, rewrite, nil, "git", "push", "--force", "origin", "rewrite:main")

	outPromote, err := runCmdAllowFailure(t, repo, device.Env, julPath, "promote", "--to", "main", "--json")
	if err == nil {
		t.Fatalf("expected promote to refuse rewritten target, got %s", outPromote)
	}
	var promoteErr output.ErrorOutput
	if err := json.NewDecoder(strings.NewReader(outPromote)).Decode(&promoteErr); err != nil {
		t.Fatalf("expected promote error json, got %v (%s)", err, outPromote)
	}
	if promoteErr.Code != "promote_target_rewritten" {
		t.Fatalf("expected promote_target_rewritten, got %+v", promoteErr)
	}
}
