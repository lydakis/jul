package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

func TestPruneRemovesExpiredKeepRefs(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "branch", "-M", "main")

	writeFilePath(t, repo, "README.md", "hello\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "base")

	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	runGitCmd(t, repo, "config", "jul.workspace", "tester/@")

	sha := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	changeID := gitutil.FallbackChangeID(sha)
	keepRef := "refs/jul/keep/tester/@/" + changeID + "/" + sha
	runGitCmd(t, repo, "update-ref", keepRef, sha)

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	if err := config.SetRepoConfigValue("retention", "checkpoint_keep_days", "0"); err != nil {
		t.Fatalf("set retention config failed: %v", err)
	}

	if _, err := metadata.WriteAttestation(client.Attestation{
		CommitSHA: sha,
		ChangeID:  changeID,
		Type:      "ci",
		Status:    "pass",
	}); err != nil {
		t.Fatalf("WriteAttestation failed: %v", err)
	}

	if code := newPruneCommand().Run([]string{}); code != 0 {
		t.Fatalf("prune failed with %d", code)
	}

	if gitutil.RefExists(keepRef) {
		t.Fatalf("expected keep-ref to be deleted")
	}
	if att, _ := metadata.GetAttestation(sha); att != nil {
		t.Fatalf("expected attestation note to be removed")
	}
}
