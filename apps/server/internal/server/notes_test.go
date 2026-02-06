package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lydakis/jul/server/internal/storage"
)

func TestWriteAttestationNoteUsesServerIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmp := t.TempDir()
	bareRepo := filepath.Join(tmp, "demo.git")
	runGit(t, tmp, "init", "--bare", bareRepo)

	cloneDir := filepath.Join(tmp, "clone")
	runGit(t, tmp, "clone", bareRepo, cloneDir)
	runGit(t, cloneDir, "config", "user.name", "Test User")
	runGit(t, cloneDir, "config", "user.email", "test@example.com")

	readme := filepath.Join(cloneDir, "README.md")
	if err := os.WriteFile(readme, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, cloneDir, "add", "README.md")
	runGit(t, cloneDir, "commit", "-m", "feat: note")
	commitSHA := strings.TrimSpace(runGitOutput(t, cloneDir, "rev-parse", "HEAD"))
	runGit(t, cloneDir, "push", "origin", "HEAD:main")

	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	t.Setenv("GIT_COMMITTER_NAME", "")
	t.Setenv("GIT_COMMITTER_EMAIL", "")

	att := storage.Attestation{
		AttestationID: "att-1",
		CommitSHA:     commitSHA,
		ChangeID:      "I0123456789abcdef0123456789abcdef01234567",
		Type:          "ci",
		Status:        "pass",
		StartedAt:     time.Now().UTC(),
		FinishedAt:    time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	if err := writeAttestationNote(bareRepo, commitSHA, att); err != nil {
		t.Fatalf("writeAttestationNote failed: %v", err)
	}

	note := strings.TrimSpace(runGitOutput(t, tmp, "--git-dir", bareRepo, "notes", "--ref", notesRef, "show", commitSHA))
	if !strings.Contains(note, `"attestation_id":"att-1"`) {
		t.Fatalf("expected attestation note payload, got %q", note)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}
