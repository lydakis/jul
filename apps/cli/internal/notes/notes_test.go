package notes

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type notePayload struct {
	Status string `json:"status"`
	When   string `json:"when"`
}

func TestNotesRoundTrip(t *testing.T) {
	repo := initRepo(t)
	commit := commitFile(t, repo, "README.md", "hello\n", "test commit")

	payload := notePayload{
		Status: "pass",
		When:   time.Now().UTC().Format(time.RFC3339),
	}
	withRepo(t, repo, func() {
		if err := AddJSON(RefAttestationsCheckpoint, commit, payload); err != nil {
			t.Fatalf("AddJSON failed: %v", err)
		}

		var out notePayload
		found, err := ReadJSON(RefAttestationsCheckpoint, commit, &out)
		if err != nil {
			t.Fatalf("ReadJSON failed: %v", err)
		}
		if !found {
			t.Fatalf("expected note to be found")
		}
		if out.Status != payload.Status {
			t.Fatalf("expected status %s, got %s", payload.Status, out.Status)
		}

		entries, err := List(RefAttestationsCheckpoint)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(entries) != 1 || entries[0].ObjectSHA != commit {
			t.Fatalf("unexpected entries: %+v", entries)
		}
	})
}

func TestListOutsideRepoReturnsJulError(t *testing.T) {
	dir := t.TempDir()
	withRepo(t, dir, func() {
		_, err := List(RefSuggestions)
		if err == nil {
			t.Fatalf("expected error outside repo")
		}
		if !errors.Is(err, ErrRepoRequired) {
			t.Fatalf("expected ErrRepoRequired, got %v", err)
		}
		if strings.Contains(strings.ToLower(err.Error()), "git") {
			t.Fatalf("expected jul-scoped error, got %q", err.Error())
		}
	})
}

func TestReadJSONOutsideRepoReturnsJulError(t *testing.T) {
	dir := t.TempDir()
	withRepo(t, dir, func() {
		var payload notePayload
		found, err := ReadJSON(RefSuggestions, "deadbeef", &payload)
		if err == nil {
			t.Fatalf("expected error outside repo")
		}
		if found {
			t.Fatalf("expected found=false outside repo")
		}
		if !errors.Is(err, ErrRepoRequired) {
			t.Fatalf("expected ErrRepoRequired, got %v", err)
		}
		if strings.Contains(strings.ToLower(err.Error()), "git") {
			t.Fatalf("expected jul-scoped error, got %q", err.Error())
		}
	})
}

func TestMissingNotesRefReturnsEmpty(t *testing.T) {
	repo := initRepo(t)
	commit := commitFile(t, repo, "README.md", "hello\n", "test commit")

	withRepo(t, repo, func() {
		var payload notePayload
		found, err := ReadJSON(RefSuggestions, commit, &payload)
		if err != nil {
			t.Fatalf("ReadJSON failed: %v", err)
		}
		if found {
			t.Fatalf("expected no note for missing ref")
		}

		entries, err := List(RefSuggestions)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected no entries for missing ref, got %+v", entries)
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
