package restack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLatestCheckpointForChangePrefersDescendantWithEqualTimestamps(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	runGit(t, repo, nil, "init")
	runGit(t, repo, nil, "config", "user.name", "Test User")
	runGit(t, repo, nil, "config", "user.email", "test@example.com")

	writeFile(t, repo, "base.txt", "base\n")
	runGit(t, repo, nil, "add", "base.txt")
	runGit(t, repo, nil, "commit", "-m", "base")

	changeID := "I1234567890abcdef1234567890abcdef1234567"
	sameTime := map[string]string{
		"GIT_AUTHOR_DATE":    "2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE": "2026-01-01T00:00:00Z",
	}

	writeFile(t, repo, "feature.txt", "one\n")
	runGit(t, repo, nil, "add", "feature.txt")
	runGit(t, repo, sameTime, "commit", "-m", "feat: one\n\nChange-Id: "+changeID)
	first := strings.TrimSpace(runGit(t, repo, nil, "rev-parse", "HEAD"))

	writeFile(t, repo, "feature.txt", "one\ntwo\n")
	runGit(t, repo, nil, "add", "feature.txt")
	runGit(t, repo, sameTime, "commit", "-m", "feat: two\n\nChange-Id: "+changeID)
	second := strings.TrimSpace(runGit(t, repo, nil, "rev-parse", "HEAD"))

	runGit(t, repo, nil, "update-ref", keepRefPrefix("tester", "@")+changeID+"/"+first, first)
	runGit(t, repo, nil, "update-ref", keepRefPrefix("tester", "@")+changeID+"/"+second, second)

	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	got, err := latestCheckpointForChange("tester", "@", changeID)
	if err != nil {
		t.Fatalf("latestCheckpointForChange failed: %v", err)
	}
	if strings.TrimSpace(got) != second {
		t.Fatalf("expected latest checkpoint %s, got %s", second, got)
	}
}

func runGit(t *testing.T, dir string, extraEnv map[string]string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		env := os.Environ()
		for key, value := range extraEnv {
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out)
}

func writeFile(t *testing.T, repoRoot, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(repoRoot, relPath)
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}
