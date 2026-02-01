//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type syncResult struct {
	DraftSHA     string `json:"DraftSHA"`
	WorkspaceRef string `json:"WorkspaceRef"`
	SyncRef      string `json:"SyncRef"`
	RemoteProblem string `json:"RemoteProblem"`
	BaseAdvanced bool   `json:"BaseAdvanced"`
}

func TestIT_DRAFT_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	writeFile(t, repo, "README.md", "base\nstaged\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	writeFile(t, repo, "README.md", "base\nstaged\nunstaged\n")
	writeFile(t, repo, "staged.txt", "staged\n")
	runCmd(t, repo, nil, "git", "add", "staged.txt")

	cachedBefore := runCmd(t, repo, nil, "git", "diff", "--cached")
	statusBefore := runCmd(t, repo, nil, "git", "status", "--porcelain")

	syncOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var res syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&res); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if res.DraftSHA == "" {
		t.Fatalf("expected draft sha")
	}

	cachedAfter := runCmd(t, repo, nil, "git", "diff", "--cached")
	statusAfter := runCmd(t, repo, nil, "git", "status", "--porcelain")

	if cachedBefore != cachedAfter {
		t.Fatalf("expected cached diff unchanged")
	}
	if statusBefore != statusAfter {
		t.Fatalf("expected status unchanged")
	}
}

func TestIT_DRAFT_002(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, "README.md", "hello\n")

	firstOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var first syncResult
	if err := json.NewDecoder(strings.NewReader(firstOut)).Decode(&first); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	secondOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var second syncResult
	if err := json.NewDecoder(strings.NewReader(secondOut)).Decode(&second); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if first.DraftSHA == "" || second.DraftSHA == "" {
		t.Fatalf("expected draft shas")
	}
	if first.DraftSHA != second.DraftSHA {
		t.Fatalf("expected draft sha to remain identical")
	}
}

func TestIT_DRAFT_004(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/notes.txt", "secret\n")
	writeFile(t, repo, "README.md", "hello\n")

	syncOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var res syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&res); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if res.DraftSHA == "" {
		t.Fatalf("expected draft sha")
	}

	files := runCmd(t, repo, nil, "git", "ls-tree", "-r", "--name-only", res.DraftSHA)
	if strings.Contains(files, ".jul/") {
		t.Fatalf("expected .jul to be excluded from draft, got: %s", files)
	}
}

func TestIT_SYNC_BASEADV_001(t *testing.T) {
	repoRoot := t.TempDir()
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})

	// Seed remote with initial repo.
	seed := filepath.Join(repoRoot, "seed")
	initRepo(t, seed, true)
	runCmd(t, seed, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, seed, nil, "git", "push", "-u", "origin", "main")

	// Clone device A and B from the same base.
	repoA := filepath.Join(repoRoot, "repoA")
	repoB := filepath.Join(repoRoot, "repoB")
	runCmd(t, repoRoot, nil, "git", "clone", remoteDir, repoA)
	runCmd(t, repoRoot, nil, "git", "clone", remoteDir, repoB)

	deviceA := newDeviceEnv(t, "devA")	
	deviceB := newDeviceEnv(t, "devB")
	julPath := buildCLI(t)

	runCmd(t, repoA, deviceA.Env, julPath, "init", "demo")
	runCmd(t, repoB, deviceB.Env, julPath, "init", "demo")

	// Device B creates a draft on the old base.
	writeFile(t, repoB, "conflict.txt", "from B\n")
	bSync := runCmd(t, repoB, deviceB.Env, julPath, "sync", "--json")
	var bRes syncResult
	if err := json.NewDecoder(strings.NewReader(bSync)).Decode(&bRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if bRes.DraftSHA == "" {
		t.Fatalf("expected draft sha on device B")
	}
	baseBefore := strings.TrimSpace(runCmd(t, repoB, nil, "git", "rev-parse", bRes.DraftSHA+"^"))

	// Device A checkpoints and advances workspace.
	writeFile(t, repoA, "conflict.txt", "from A\n")
	runCmd(t, repoA, deviceA.Env, julPath, "checkpoint", "-m", "feat: A", "--no-ci", "--no-review", "--json")

	// Device B syncs again and should detect base advanced.
	bSync2 := runCmd(t, repoB, deviceB.Env, julPath, "sync", "--json")
	var bRes2 syncResult
	if err := json.NewDecoder(strings.NewReader(bSync2)).Decode(&bRes2); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if !bRes2.BaseAdvanced && !strings.Contains(bRes2.RemoteProblem, "base advanced") {
		t.Fatalf("expected base advanced warning, got %+v", bRes2)
	}
	baseAfter := strings.TrimSpace(runCmd(t, repoB, nil, "git", "rev-parse", bRes2.DraftSHA+"^"))
	if baseBefore != baseAfter {
		t.Fatalf("expected draft base to remain unchanged, got %s vs %s", baseBefore, baseAfter)
	}
}
