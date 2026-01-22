package ci

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigCommandsOrder(t *testing.T) {
	repo := initRepo(t)
	path := filepath.Join(repo, ".jul", "ci.toml")
	content := `[commands]
lint = "ruff check ."
test = "pytest"
coverage = "pytest --cov"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	withRepo(t, repo, func() {
		cfg, ok, err := LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}
		if !ok {
			t.Fatalf("expected config to be found")
		}
		if len(cfg.Commands) != 3 {
			t.Fatalf("expected 3 commands, got %d", len(cfg.Commands))
		}
		expected := []string{"ruff check .", "pytest", "pytest --cov"}
		for i, cmd := range expected {
			if cfg.Commands[i] != cmd {
				t.Fatalf("expected command %q at %d, got %q", cmd, i, cfg.Commands[i])
			}
		}
	})
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "git", "init")
	if err := os.MkdirAll(filepath.Join(repo, ".jul"), 0o755); err != nil {
		t.Fatalf("failed to create .jul dir: %v", err)
	}
	return repo
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
