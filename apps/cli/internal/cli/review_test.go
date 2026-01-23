package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewRootCommitDiff(t *testing.T) {
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "user.email", "test@example.com")

	file := filepath.Join(repo, "README.md")
	if err := os.WriteFile(file, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	runGitTest(t, repo, "add", "README.md")
	runGitTest(t, repo, "commit", "-m", "initial")

	sha := runGitOutputTest(t, repo, "rev-parse", "HEAD")
	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	diff := reviewDiff(strings.TrimSpace(sha))
	if strings.TrimSpace(diff) == "" {
		t.Fatalf("expected non-empty diff for root commit")
	}
	files := reviewFiles(strings.TrimSpace(sha))
	if len(files) == 0 {
		t.Fatalf("expected files for root commit")
	}
	found := false
	for _, f := range files {
		if f.Path == "README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected README.md in review files")
	}
}

func TestAutoCommitWorktreeSkipsReviewAttachment(t *testing.T) {
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("write keep file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "jul-review-123.txt"), []byte("context\n"), 0o644); err != nil {
		t.Fatalf("write attachment file failed: %v", err)
	}

	commit, err := autoCommitWorktree(repo, "agent: review")
	if err != nil {
		t.Fatalf("autoCommitWorktree failed: %v", err)
	}
	if strings.TrimSpace(commit) == "" {
		t.Fatalf("expected commit sha")
	}

	paths := runGitOutputTest(t, repo, "show", "--pretty=format:", "--name-only", "HEAD")
	if strings.Contains(paths, "jul-review-123.txt") {
		t.Fatalf("attachment file should not be committed: %s", paths)
	}
	if !strings.Contains(paths, "keep.txt") {
		t.Fatalf("expected keep.txt to be committed: %s", paths)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}

func runGitOutputTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
	return strings.TrimSpace(string(output))
}
