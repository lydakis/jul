package syncer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestScrubSecrets(t *testing.T) {
	inputs := []string{
		"Bearer abc123token",
		"api_key=secretvalue",
		"token: supersecret",
		"sk-1234567890abcdef",
		"ghp_1234567890abcdef1234567890abcdef",
	}
	for _, input := range inputs {
		out := scrubSecrets(input)
		if out == input {
			t.Fatalf("expected scrubbed output for %q", input)
		}
		if out == "" {
			t.Fatalf("expected non-empty scrubbed output")
		}
	}
}

func TestTraceMergeUsesWorkspaceTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("JUL_WORKSPACE", "tester/@")

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "base.txt"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	baseSHA, err := gitOut(repoDir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	baseTree, err := gitOut(repoDir, "git", "rev-parse", baseSHA+"^{tree}")
	if err != nil {
		t.Fatal(err)
	}

	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(repoDir, "git", "update-ref", workspaceRef, baseSHA); err != nil {
		t.Fatal(err)
	}

	traceA, err := gitOut(repoDir, "git", "commit-tree", baseTree, "-m", "[trace]")
	if err != nil {
		t.Fatal(err)
	}
	traceRef := "refs/jul/traces/tester/@"
	if err := run(repoDir, "git", "update-ref", traceRef, traceA); err != nil {
		t.Fatal(err)
	}

	deviceID, err := config.DeviceID()
	if err != nil {
		t.Fatal(err)
	}
	traceSyncRef := "refs/jul/trace-sync/tester/" + deviceID + "/@"

	if err := os.WriteFile(filepath.Join(repoDir, "base.txt"), []byte("draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	draftTree, err := gitutil.DraftTree()
	if err != nil {
		t.Fatal(err)
	}
	traceB, err := gitOut(repoDir, "git", "commit-tree", draftTree, "-m", "[trace]")
	if err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "update-ref", traceSyncRef, traceB); err != nil {
		t.Fatal(err)
	}

	res, err := Trace(TraceOptions{Force: true, UpdateCanonical: true})
	if err != nil {
		t.Fatalf("trace failed: %v", err)
	}
	if res.CanonicalSHA == "" {
		t.Fatalf("expected canonical trace sha")
	}
	mergeTree, err := gitOut(repoDir, "git", "rev-parse", res.CanonicalSHA+"^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	if mergeTree != baseTree {
		t.Fatalf("expected merge trace tree %s to match workspace tree %s", mergeTree, baseTree)
	}
}
