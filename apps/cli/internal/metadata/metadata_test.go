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

		pending, err := ListSuggestions("", "pending", 10)
		if err != nil {
			t.Fatalf("ListSuggestions failed: %v", err)
		}
		if len(pending) != 1 {
			t.Fatalf("expected 1 pending suggestion, got %d", len(pending))
		}

		updated, err := UpdateSuggestionStatus(sug.SuggestionID, "applied", "")
		if err != nil {
			t.Fatalf("UpdateSuggestionStatus failed: %v", err)
		}
		if updated.Status != "applied" {
			t.Fatalf("expected applied status, got %s", updated.Status)
		}

		foundSug, ok, err := GetSuggestionByID(sug.SuggestionID)
		if err != nil {
			t.Fatalf("GetSuggestionByID failed: %v", err)
		}
		if !ok || foundSug.SuggestionID != sug.SuggestionID {
			t.Fatalf("expected to find suggestion by id")
		}

		accepted, err := ListSuggestions("", "applied", 10)
		if err != nil {
			t.Fatalf("ListSuggestions failed: %v", err)
		}
		if len(accepted) != 1 {
			t.Fatalf("expected 1 applied suggestion, got %d", len(accepted))
		}
	})
}

func TestTraceNoteRoundTrip(t *testing.T) {
	repo := initRepo(t)
	traceSHA := commitFile(t, repo, "README.md", "hello\n", "trace commit")

	withRepo(t, repo, func() {
		note := TraceNote{
			TraceSHA:   traceSHA,
			PromptHash: "sha256:deadbeef",
			Agent:      "test-agent",
			SessionID:  "session-1",
			Turn:       2,
			Device:     "device-1",
		}
		if err := WriteTrace(note); err != nil {
			t.Fatalf("WriteTrace failed: %v", err)
		}
		got, err := GetTrace(traceSHA)
		if err != nil {
			t.Fatalf("GetTrace failed: %v", err)
		}
		if got == nil {
			t.Fatalf("expected trace note")
		}
		if got.PromptHash != note.PromptHash {
			t.Fatalf("expected prompt hash %q, got %q", note.PromptHash, got.PromptHash)
		}
		if err := WriteTracePrompt(traceSHA, "add auth"); err != nil {
			t.Fatalf("WriteTracePrompt failed: %v", err)
		}
		if err := WriteTraceSummary(traceSHA, "Added auth"); err != nil {
			t.Fatalf("WriteTraceSummary failed: %v", err)
		}
		prompt, err := ReadTracePrompt(traceSHA)
		if err != nil {
			t.Fatalf("ReadTracePrompt failed: %v", err)
		}
		if prompt != "add auth" {
			t.Fatalf("expected prompt %q, got %q", "add auth", prompt)
		}
		summary, err := ReadTraceSummary(traceSHA)
		if err != nil {
			t.Fatalf("ReadTraceSummary failed: %v", err)
		}
		if summary != "Added auth" {
			t.Fatalf("expected summary %q, got %q", "Added auth", summary)
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
