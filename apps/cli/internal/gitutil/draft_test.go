package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectChangedPathsFromStatus(t *testing.T) {
	status := " M README.md\x00?? newdir/\x00R  dst.txt\x00src.txt\x00"
	paths := collectChangedPathsFromStatus(status)
	if len(paths) != 4 {
		t.Fatalf("expected 4 paths, got %d (%v)", len(paths), paths)
	}

	expected := map[string]bool{
		"README.md": true,
		"newdir/":   true,
		"dst.txt":   true,
		"src.txt":   true,
	}
	for _, path := range paths {
		if !expected[path] {
			t.Fatalf("unexpected path %q in %v", path, paths)
		}
		delete(expected, path)
	}
	if len(expected) != 0 {
		t.Fatalf("missing paths: %v", expected)
	}
}

func TestCollectChangedPathsFromStatusIgnoresInvalidEntries(t *testing.T) {
	status := "\x00 \x00?? file.txt\x00"
	paths := collectChangedPathsFromStatus(status)
	if len(paths) != 1 || paths[0] != "file.txt" {
		t.Fatalf("expected only file.txt, got %v", paths)
	}
}

func TestUpdateIndexIncrementalStagesTrackedModifications(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readmePath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")

	julDir := filepath.Join(repo, ".jul")
	if err := os.MkdirAll(julDir, 0o755); err != nil {
		t.Fatalf("mkdir .jul: %v", err)
	}
	indexPath := filepath.Join(julDir, "draft-index")
	excludePath, err := writeTempExcludes(repo)
	if err != nil {
		t.Fatalf("writeTempExcludes: %v", err)
	}
	defer os.Remove(excludePath)

	env := map[string]string{"GIT_INDEX_FILE": indexPath}
	if err := runGitWithEnv(repo, env, "-c", "core.excludesfile="+excludePath, "add", "-A", "--", "."); err != nil {
		t.Fatalf("seed draft index: %v", err)
	}

	if err := os.WriteFile(readmePath, []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("update README: %v", err)
	}

	if err := updateIndexIncremental(repo, indexPath, excludePath); err != nil {
		t.Fatalf("updateIndexIncremental failed: %v", err)
	}

	staged, err := gitWithEnv(repo, env, "diff", "--cached", "--name-only")
	if err != nil {
		t.Fatalf("cached diff failed: %v", err)
	}
	if strings.TrimSpace(staged) != "README.md" {
		t.Fatalf("expected staged path README.md, got %q", staged)
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
