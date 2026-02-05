package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRefHeadAndBranch(t *testing.T) {
	repo := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.name failed: %v", err)
	}
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.email failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "test commit")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v", err)
	}
	headSHA := strings.TrimSpace(string(out))

	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repo
	out, err = cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --abbrev-ref HEAD failed: %v", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		t.Fatalf("empty branch")
	}

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	headInfo, err := ReadHeadInfo()
	if err != nil {
		t.Fatalf("HeadInfo failed: %v", err)
	}
	if strings.TrimSpace(headInfo.SHA) != headSHA {
		t.Fatalf("HeadInfo SHA mismatch: %s vs %s", headInfo.SHA, headSHA)
	}
	if strings.TrimSpace(headInfo.Branch) != branch {
		t.Fatalf("HeadInfo branch mismatch: %s vs %s", headInfo.Branch, branch)
	}

	resolvedHead, err := ResolveRef("HEAD")
	if err != nil {
		t.Fatalf("ResolveRef HEAD failed: %v", err)
	}
	if strings.TrimSpace(resolvedHead) != headSHA {
		t.Fatalf("ResolveRef HEAD mismatch: %s vs %s", resolvedHead, headSHA)
	}

	fullRef := "refs/heads/" + branch
	resolvedBranch, err := ResolveRef(fullRef)
	if err != nil {
		t.Fatalf("ResolveRef %s failed: %v", fullRef, err)
	}
	if strings.TrimSpace(resolvedBranch) != headSHA {
		t.Fatalf("ResolveRef %s mismatch: %s vs %s", fullRef, resolvedBranch, headSHA)
	}
}
