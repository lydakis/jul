//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/output"
)

type syncResult struct {
	DraftSHA         string   `json:"DraftSHA"`
	WorkspaceRef     string   `json:"WorkspaceRef"`
	SyncRef          string   `json:"SyncRef"`
	RemoteProblem    string   `json:"RemoteProblem"`
	BaseAdvanced     bool     `json:"BaseAdvanced"`
	WorkspaceUpdated bool     `json:"WorkspaceUpdated"`
	Diverged         bool     `json:"Diverged"`
	Warnings         []string `json:"Warnings"`
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

	readme := readFile(t, repo, "README.md")
	draftReadme := runCmd(t, repo, nil, "git", "show", res.DraftSHA+":README.md")
	if readme != draftReadme {
		t.Fatalf("expected draft README to match working tree")
	}
	staged := readFile(t, repo, "staged.txt")
	draftStaged := runCmd(t, repo, nil, "git", "show", res.DraftSHA+":staged.txt")
	if staged != draftStaged {
		t.Fatalf("expected draft staged.txt to match working tree")
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
	writeFile(t, repoB, ".jul/config.toml", "[sync]\nautorestack = false\n")

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

	promoteOut, err := runCmdAllowFailure(t, repoB, deviceB.Env, julPath, "promote", "--to", "main", "--json")
	if err == nil {
		t.Fatalf("expected promote to be blocked after base advance, got %s", promoteOut)
	}
	var promoteErr output.ErrorOutput
	if err := json.NewDecoder(strings.NewReader(promoteOut)).Decode(&promoteErr); err != nil {
		t.Fatalf("expected promote error json, got %v (%s)", err, promoteOut)
	}
	if !strings.Contains(strings.ToLower(promoteErr.Message), "restack") && !strings.Contains(strings.ToLower(promoteErr.Message), "checkout") {
		t.Fatalf("expected promote to suggest restack/checkout, got %+v", promoteErr)
	}
}

func TestIT_SYNC_AUTORESTACK_001(t *testing.T) {
	root := t.TempDir()
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})

	seed := filepath.Join(root, "seed")
	initRepo(t, seed, true)
	runCmd(t, seed, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, seed, nil, "git", "push", "-u", "origin", "main")

	repoA := filepath.Join(root, "repoA")
	repoB := filepath.Join(root, "repoB")
	runCmd(t, root, nil, "git", "clone", remoteDir, repoA)
	runCmd(t, root, nil, "git", "clone", remoteDir, repoB)

	deviceA := newDeviceEnv(t, "devA")
	deviceB := newDeviceEnv(t, "devB")
	julPath := buildCLI(t)

	runCmd(t, repoA, deviceA.Env, julPath, "init", "demo")
	runCmd(t, repoB, deviceB.Env, julPath, "init", "demo")

	writeFile(t, repoB, "b.txt", "from B\n")
	cpBOut := runCmd(t, repoB, deviceB.Env, julPath, "checkpoint", "-m", "feat: B", "--no-ci", "--no-review", "--json")
	var cpB checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpBOut)).Decode(&cpB); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}

	runCmd(t, repoA, deviceA.Env, julPath, "sync", "--json")
	writeFile(t, repoA, "a.txt", "from A\n")
	cpAOut := runCmd(t, repoA, deviceA.Env, julPath, "checkpoint", "-m", "feat: A", "--no-ci", "--no-review", "--json")
	var cpA checkpointResult
	if err := json.NewDecoder(strings.NewReader(cpAOut)).Decode(&cpA); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}

	syncOut := runCmd(t, repoB, deviceB.Env, julPath, "sync", "--json")
	var res syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&res); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if res.Diverged || res.BaseAdvanced {
		t.Fatalf("expected autorestack to resolve base advanced, got %+v", res)
	}
	if !res.WorkspaceUpdated {
		t.Fatalf("expected workspace updated after autorestack")
	}

	keepPrefix := "refs/jul/keep/tester/@/" + cpB.ChangeID + "/"
	keepOut := runCmd(t, repoB, nil, "git", "for-each-ref", "--format=%(objectname) %(refname)", keepPrefix)
	keepLines := strings.Fields(strings.TrimSpace(keepOut))
	if len(keepLines) < 4 {
		t.Fatalf("expected at least two keep refs after restack, got %s", keepOut)
	}
	workspaceTip := strings.TrimSpace(runCmd(t, repoB, nil, "git", "rev-parse", "refs/jul/workspaces/tester/@"))
	if workspaceTip == cpB.CheckpointSHA {
		t.Fatalf("expected workspace tip to move after restack")
	}
	foundWorkspace := false
	for i := 0; i+1 < len(keepLines); i += 2 {
		if strings.TrimSpace(keepLines[i]) == workspaceTip {
			foundWorkspace = true
			break
		}
	}
	if !foundWorkspace {
		t.Fatalf("expected workspace tip %s to be kept, got %s", workspaceTip, keepOut)
	}

	draftParent := strings.TrimSpace(runCmd(t, repoB, nil, "git", "rev-parse", res.DraftSHA+"^"))
	if draftParent != workspaceTip {
		t.Fatalf("expected draft parent to match workspace tip, got %s vs %s", draftParent, workspaceTip)
	}
	if draftParent == cpB.CheckpointSHA {
		t.Fatalf("expected restacked checkpoint to differ from %s", cpB.CheckpointSHA)
	}
	parentParent := strings.TrimSpace(runCmd(t, repoB, nil, "git", "rev-parse", draftParent+"^"))
	if parentParent != cpA.CheckpointSHA {
		t.Fatalf("expected restack parent %s, got %s", cpA.CheckpointSHA, parentParent)
	}
}

func TestIT_SYNC_AUTORESTACK_002(t *testing.T) {
	root := t.TempDir()
	remoteDir := newRemoteSimulator(t, remoteConfig{Mode: remoteFullCompat})

	seed := filepath.Join(root, "seed")
	initRepo(t, seed, true)
	runCmd(t, seed, nil, "git", "remote", "add", "origin", remoteDir)
	runCmd(t, seed, nil, "git", "push", "-u", "origin", "main")

	repoA := filepath.Join(root, "repoA")
	repoB := filepath.Join(root, "repoB")
	runCmd(t, root, nil, "git", "clone", remoteDir, repoA)
	runCmd(t, root, nil, "git", "clone", remoteDir, repoB)

	deviceA := newDeviceEnv(t, "devA")
	deviceB := newDeviceEnv(t, "devB")
	julPath := buildCLI(t)

	runCmd(t, repoA, deviceA.Env, julPath, "init", "demo")
	runCmd(t, repoB, deviceB.Env, julPath, "init", "demo")

	writeFile(t, repoB, "conflict.txt", "from B\n")
	runCmd(t, repoB, deviceB.Env, julPath, "checkpoint", "-m", "feat: B", "--no-ci", "--no-review", "--json")

	runCmd(t, repoA, deviceA.Env, julPath, "sync", "--json")
	writeFile(t, repoA, "conflict.txt", "from A\n")
	runCmd(t, repoA, deviceA.Env, julPath, "checkpoint", "-m", "feat: A", "--no-ci", "--no-review", "--json")

	syncOut := runCmd(t, repoB, deviceB.Env, julPath, "sync", "--json")
	var res syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&res); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if !res.Diverged || !strings.Contains(res.RemoteProblem, "restack conflict") {
		t.Fatalf("expected restack conflict, got %+v", res)
	}

}
