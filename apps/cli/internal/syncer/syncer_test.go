package syncer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeepRefPathIncludesUser(t *testing.T) {
	got := keepRefPath("george", "@", "Iabc", "def")
	want := "refs/jul/keep/george/@/Iabc/def"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCheckpointErrorsOnKeepRefPushFailure(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := run(repoDir, "git", "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(tmp, "remote.git")
	if err := run(tmp, "git", "init", "--bare", remoteDir); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(remoteDir, "hooks", "update")
	hook := "#!/bin/sh\nrefname=\"$1\"\ncase \"$refname\" in\n  refs/jul/keep/*) echo \"deny keep\" >&2; exit 1 ;;\nesac\nexit 0\n"
	if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := run(repoDir, "git", "remote", "add", "origin", remoteDir); err != nil {
		t.Fatal(err)
	}
	workspaceRef := "refs/jul/workspaces/tester/@"
	if err := run(repoDir, "git", "push", "origin", "HEAD:"+workspaceRef); err != nil {
		t.Fatal(err)
	}
	basePath := filepath.Join(repoDir, ".jul", "workspaces", "@", "base")
	if err := os.MkdirAll(filepath.Dir(basePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(basePath, []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if _, err := Checkpoint("feat: test"); err == nil {
		t.Fatalf("expected keep-ref push error, got nil")
	} else if !strings.Contains(err.Error(), "deny keep") && !strings.Contains(err.Error(), "refs/jul/keep") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &execError{cmd: name + " " + strings.Join(args, " "), output: string(out)}
	}
	return nil
}

type execError struct {
	cmd    string
	output string
}

func (e *execError) Error() string {
	return strings.TrimSpace(e.cmd + ": " + e.output)
}
