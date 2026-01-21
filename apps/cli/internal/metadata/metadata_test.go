package metadata

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func TestAttestationRoundTrip(t *testing.T) {
	repo := initRepo(t)
	commit := commitFile(t, repo, "README.md", "hello\n", "test commit")

	withRepo(t, repo, func() {
		att := client.Attestation{
			CommitSHA: commit,
			ChangeID:  gitutil.FallbackChangeID(commit),
			Type:      "ci",
			Status:    "pass",
		}
		created, err := WriteAttestation(att)
		if err != nil {
			t.Fatalf("WriteAttestation failed: %v", err)
		}
		got, err := GetAttestation(commit)
		if err != nil {
			t.Fatalf("GetAttestation failed: %v", err)
		}
		if got == nil || got.AttestationID == "" {
			t.Fatalf("expected attestation to be stored")
		}
		if got.Status != created.Status {
			t.Fatalf("expected status %s, got %s", created.Status, got.Status)
		}
	})
}

func TestSuggestionLifecycle(t *testing.T) {
	repo := initRepo(t)
	base := commitFile(t, repo, "README.md", "hello\n", "base commit")
	suggested := commitFile(t, repo, "README.md", "hello\nworld\n", "suggested commit")

	withRepo(t, repo, func() {
		sug, err := CreateSuggestion(SuggestionCreate{
			BaseCommitSHA:      base,
			SuggestedCommitSHA: suggested,
			CreatedBy:          "tester",
			Reason:             "fix",
		})
		if err != nil {
			t.Fatalf("CreateSuggestion failed: %v", err)
		}
		if sug.SuggestionID == "" || sug.ChangeID == "" {
			t.Fatalf("expected suggestion ids")
		}

		ref := "refs/jul/suggest/" + sug.ChangeID + "/" + sug.SuggestionID
		resolved, err := gitutil.ResolveRef(ref)
		if err != nil {
			t.Fatalf("ResolveRef failed: %v", err)
		}
		if strings.TrimSpace(resolved) != suggested {
			t.Fatalf("expected ref to point at %s, got %s", suggested, resolved)
		}

		open, err := ListSuggestions("", "open", 10)
		if err != nil {
			t.Fatalf("ListSuggestions failed: %v", err)
		}
		if len(open) != 1 {
			t.Fatalf("expected 1 open suggestion, got %d", len(open))
		}

		updated, err := UpdateSuggestionStatus(sug.SuggestionID, "accepted")
		if err != nil {
			t.Fatalf("UpdateSuggestionStatus failed: %v", err)
		}
		if updated.Status != "accepted" {
			t.Fatalf("expected accepted status, got %s", updated.Status)
		}

		foundSug, ok, err := GetSuggestionByID(sug.SuggestionID)
		if err != nil {
			t.Fatalf("GetSuggestionByID failed: %v", err)
		}
		if !ok || foundSug.SuggestionID != sug.SuggestionID {
			t.Fatalf("expected to find suggestion by id")
		}

		accepted, err := ListSuggestions("", "accepted", 10)
		if err != nil {
			t.Fatalf("ListSuggestions failed: %v", err)
		}
		if len(accepted) != 1 {
			t.Fatalf("expected 1 accepted suggestion, got %d", len(accepted))
		}
	})
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	return repo
}

func commitFile(t *testing.T, repo, name, content, message string) string {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	run(t, repo, "git", "add", name)
	run(t, repo, "git", "commit", "-m", message)
	out := run(t, repo, "git", "rev-parse", "HEAD")
	return out
}

func withRepo(t *testing.T, repo string, fn func()) {
	t.Helper()
	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	fn()
}

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %v\n%s", name, args, string(out))
	}
	return strings.TrimSpace(string(out))
}
