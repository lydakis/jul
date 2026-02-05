package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildCLI(t *testing.T) string {
	t.Helper()
	root := findRepoRoot(t)
	cliDir := filepath.Join(root, "apps", "cli")
	outPath := filepath.Join(t.TempDir(), "jul")

	cmd := exec.Command("go", "build", "-o", outPath, "./cmd/jul")
	cmd.Dir = cliDir
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(output))
	}
	return outPath
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("failed to locate repo root from %s", cwd)
	return ""
}

func runCmd(t *testing.T, dir string, env map[string]string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s\n%s", name, strings.Join(args, " "), string(output))
	}
	return string(output)
}

func runCmdAllowFailure(t *testing.T, dir string, env map[string]string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

func mergeEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return append(os.Environ(), "JUL_NO_SYNC=1")
	}

	env := os.Environ()
	if _, ok := extra["JUL_NO_SYNC"]; !ok {
		if _, hook := extra["JUL_HOOK_CMD"]; !hook {
			env = append(env, "JUL_NO_SYNC=1")
		}
	}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	return string(data)
}
