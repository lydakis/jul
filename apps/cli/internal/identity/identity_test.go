package identity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

func TestResolveUserNamespaceFromRepoMeta(t *testing.T) {
	repo := initRepo(t)
	chdir(t, repo)

	root, _ := gitutil.RootCommit()
	meta := metadata.RepoMeta{RepoID: "jul:deadbeef", UserNamespace: "alice-1234"}
	if err := metadata.WriteRepoMeta(root, meta); err != nil {
		t.Fatalf("write repo meta: %v", err)
	}

	ns, err := ResolveUserNamespace("")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ns != meta.UserNamespace {
		t.Fatalf("expected %q got %q", meta.UserNamespace, ns)
	}
	if cfg := config.UserNamespace(); cfg != meta.UserNamespace {
		t.Fatalf("expected config %q got %q", meta.UserNamespace, cfg)
	}
}

func TestResolveUserNamespaceFromConfig(t *testing.T) {
	repo := initRepo(t)
	chdir(t, repo)

	if err := config.SetRepoConfigValue("user", "user_namespace", "cached-7777"); err != nil {
		t.Fatalf("set config: %v", err)
	}
	ns, err := ResolveUserNamespace("")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ns != "cached-7777" {
		t.Fatalf("expected cached namespace, got %q", ns)
	}
}

func TestResolveUserNamespaceGeneratesAndPublishes(t *testing.T) {
	repo := initRepo(t)
	chdir(t, repo)

	ns, err := ResolveUserNamespace("")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ns == "" {
		t.Fatalf("expected namespace")
	}

	root, _ := gitutil.RootCommit()
	meta, ok, err := metadata.ReadRepoMeta(root)
	if err != nil {
		t.Fatalf("read repo meta: %v", err)
	}
	if !ok {
		t.Fatalf("expected repo meta note")
	}
	if meta.UserNamespace != ns {
		t.Fatalf("expected repo meta namespace %q got %q", ns, meta.UserNamespace)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := runGit(repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runGit(repo, "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if err := runGit(repo, "config", "user.name", "Test User"); err != nil {
		t.Fatalf("config name: %v", err)
	}
	filePath := filepath.Join(repo, "file.txt")
	if err := os.WriteFile(filePath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := runGit(repo, "add", "file.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := runGit(repo, "commit", "-m", "base"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return repo
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %s", args, strings.TrimSpace(string(out)))
	}
	return nil
}
