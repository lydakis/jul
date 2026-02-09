//go:build jul_integ_spec

package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type deviceEnv struct {
	Home string
	XDG  string
	Env  map[string]string
}

type remoteMode string

type remoteConfig struct {
	Mode             remoteMode
	FFOnlyPrefixes   []string
	BlockPrefixes    []string
	FlakyEvery       int
	RejectNotes      bool
	RejectCustomRefs bool
}

const (
	remoteFullCompat remoteMode = "full"
	remoteNoCustom   remoteMode = "nocustom"
	remoteNotesBlock remoteMode = "notesblocked"
	remoteFFDraft    remoteMode = "ffdraft"
	remoteSelective  remoteMode = "selective"
	remoteFlaky      remoteMode = "flaky"
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
	installBundledOpenCode(t, outPath)
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
	logCmd(t, dir, name, args)
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
	logCmd(t, dir, name, args)
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil && shouldLogCmd() {
		t.Logf("output: %s", string(output))
	}
	return string(output), err
}

func runCmdInput(t *testing.T, dir string, env map[string]string, input string, name string, args ...string) (string, error) {
	t.Helper()
	logCmd(t, dir, name, args)
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(env)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func mergeEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	env := os.Environ()
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

func logCmd(t *testing.T, dir, name string, args []string) {
	t.Helper()
	if !shouldLogCmd() {
		return
	}
	t.Logf("run: (dir=%s) %s %s", dir, name, strings.Join(args, " "))
}

func shouldLogCmd() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("JUL_IT_VERBOSE")))
	return value == "1" || value == "true" || value == "yes"
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

func initRepo(t *testing.T, repo string, withCommit bool) {
	t.Helper()
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")
	if withCommit {
		writeFile(t, repo, "README.md", "base\n")
		runCmd(t, repo, nil, "git", "add", "README.md")
		runCmd(t, repo, nil, "git", "commit", "-m", "base")
		runCmd(t, repo, nil, "git", "branch", "-M", "main")
	}
}

func newDeviceEnv(t *testing.T, name string) deviceEnv {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, name, "home")
	xdg := filepath.Join(root, name, "xdg")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("failed to create home: %v", err)
	}
	if err := os.MkdirAll(xdg, 0o755); err != nil {
		t.Fatalf("failed to create xdg: %v", err)
	}
	seedOpenCodeConfig(t, xdg)
	env := map[string]string{
		"HOME":                home,
		"XDG_CONFIG_HOME":     xdg,
		"JUL_WORKSPACE":       "tester/@",
		"OPENCODE_PERMISSION": `{"*":"allow"}`,
	}
	return deviceEnv{Home: home, XDG: xdg, Env: env}
}

func installBundledOpenCode(t *testing.T, julPath string) {
	t.Helper()
	root := findRepoRoot(t)
	opencodePath := findBundledOpenCode(t, root)
	libexecDir := filepath.Join(filepath.Dir(julPath), "libexec", "jul")
	if err := os.MkdirAll(libexecDir, 0o755); err != nil {
		t.Fatalf("failed to create libexec dir: %v", err)
	}
	dst := filepath.Join(libexecDir, opencodeBinaryName())
	if err := copyFile(opencodePath, dst); err != nil {
		t.Fatalf("failed to copy opencode: %v", err)
	}
}

func findBundledOpenCode(t *testing.T, root string) string {
	t.Helper()
	bin := opencodeBinaryName()
	platformDir := runtime.GOOS + "_" + runtime.GOARCH
	candidates := []string{
		filepath.Join(root, "libexec", "jul", platformDir, bin),
		filepath.Join(root, "libexec", "jul", bin),
		filepath.Join(root, "build", "opencode", platformDir, bin),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	t.Fatalf("opencode binary not found; looked for %s", strings.Join(candidates, ", "))
	return ""
}

func opencodeBinaryName() string {
	if runtime.GOOS == "windows" {
		return "opencode.exe"
	}
	return "opencode"
}

func seedOpenCodeConfig(t *testing.T, xdg string) {
	t.Helper()
	hostXDG := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if hostXDG == "" {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			hostXDG = filepath.Join(home, ".config")
		}
	}
	if hostXDG == "" {
		return
	}
	src := filepath.Join(hostXDG, "opencode")
	info, err := os.Stat(src)
	if err != nil {
		return
	}
	dst := filepath.Join(xdg, "opencode")
	if info.IsDir() {
		if err := copyDir(src, dst); err != nil {
			t.Fatalf("failed to seed opencode config: %v", err)
		}
		return
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("failed to seed opencode config file: %v", err)
	}
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		info, err := os.Lstat(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

func readDeviceID(t *testing.T, home string) string {
	t.Helper()
	path := filepath.Join(home, ".config", "jul", "device")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read device id: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func newRemoteSimulator(t *testing.T, cfg remoteConfig) string {
	t.Helper()
	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	runCmd(t, t.TempDir(), nil, "git", "init", "--bare", remoteDir)
	runCmd(t, remoteDir, nil, "git", "symbolic-ref", "HEAD", "refs/heads/main")
	if err := installRemoteHook(remoteDir, cfg); err != nil {
		t.Fatalf("failed to install remote hook: %v", err)
	}
	return remoteDir
}

func installRemoteHook(remoteDir string, cfg remoteConfig) error {
	hooksDir := filepath.Join(remoteDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	configPath := filepath.Join(hooksDir, "jul-hook-config")
	var b strings.Builder
	mode := cfg.Mode
	if mode == "" {
		mode = remoteFullCompat
	}
	b.WriteString("MODE=\"" + string(mode) + "\"\n")
	if len(cfg.FFOnlyPrefixes) > 0 {
		b.WriteString("FF_ONLY_PREFIXES=\"" + strings.Join(cfg.FFOnlyPrefixes, ",") + "\"\n")
	}
	if len(cfg.BlockPrefixes) > 0 {
		b.WriteString("BLOCK_PREFIXES=\"" + strings.Join(cfg.BlockPrefixes, ",") + "\"\n")
	}
	if cfg.FlakyEvery > 0 {
		b.WriteString(fmt.Sprintf("FLAKY_EVERY=%d\n", cfg.FlakyEvery))
	}
	if cfg.RejectNotes {
		b.WriteString("REJECT_NOTES=1\n")
	}
	if cfg.RejectCustomRefs {
		b.WriteString("REJECT_CUSTOM=1\n")
	}
	if err := os.WriteFile(configPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	preReceive := filepath.Join(hooksDir, "pre-receive")
	script := `#!/bin/sh
set -e
config="$GIT_DIR/hooks/jul-hook-config"
MODE="full"
FF_ONLY_PREFIXES=""
BLOCK_PREFIXES=""
FLAKY_EVERY=0
REJECT_NOTES=0
REJECT_CUSTOM=0
if [ -f "$config" ]; then
  . "$config"
fi

prefix_match() {
  ref="$1"
  prefix="$2"
  case "$ref" in
    "$prefix"*) return 0 ;;
    *) return 1 ;;
  esac
}

is_non_ff() {
  old="$1"
  new="$2"
  if [ "$old" = "0000000000000000000000000000000000000000" ]; then
    return 1
  fi
  if [ "$new" = "0000000000000000000000000000000000000000" ]; then
    return 1
  fi
  if git merge-base --is-ancestor "$old" "$new" >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

maybe_flaky() {
  if [ "$MODE" != "flaky" ]; then
    return 0
  fi
  if [ "$FLAKY_EVERY" -le 0 ]; then
    return 0
  fi
  count_file="$GIT_DIR/hooks/jul-hook-flaky-count"
  count=0
  if [ -f "$count_file" ]; then
    count=$(cat "$count_file" 2>/dev/null || echo 0)
  fi
  count=$((count + 1))
  echo "$count" > "$count_file"
  if [ $((count % FLAKY_EVERY)) -eq 0 ]; then
    echo "flaky remote failure" >&2
    exit 1
  fi
}

maybe_flaky

while read old new ref; do
  if [ "$REJECT_CUSTOM" -eq 1 ]; then
    if prefix_match "$ref" "refs/jul/"; then
      echo "custom refs blocked" >&2
      exit 1
    fi
  fi
  if [ "$REJECT_NOTES" -eq 1 ]; then
    if prefix_match "$ref" "refs/notes/jul/"; then
      echo "notes blocked" >&2
      exit 1
    fi
  fi

  case "$MODE" in
    nocustom)
      if prefix_match "$ref" "refs/jul/"; then
        echo "custom refs blocked" >&2
        exit 1
      fi
      if prefix_match "$ref" "refs/notes/jul/"; then
        echo "notes blocked" >&2
        exit 1
      fi
      ;;
    notesblocked)
      if prefix_match "$ref" "refs/notes/jul/"; then
        echo "notes blocked" >&2
        exit 1
      fi
      ;;
    ffdraft)
      if prefix_match "$ref" "refs/jul/sync/"; then
        if is_non_ff "$old" "$new"; then
          echo "non-ff draft blocked" >&2
          exit 1
        fi
      fi
      ;;
    selective)
      if [ -n "$BLOCK_PREFIXES" ]; then
        IFS=","; for prefix in $BLOCK_PREFIXES; do
          if [ -n "$prefix" ] && prefix_match "$ref" "$prefix"; then
            echo "blocked by policy" >&2
            exit 1
          fi
        done
        unset IFS
      fi
      if [ -n "$FF_ONLY_PREFIXES" ]; then
        IFS=","; for prefix in $FF_ONLY_PREFIXES; do
          if [ -n "$prefix" ] && prefix_match "$ref" "$prefix"; then
            if is_non_ff "$old" "$new"; then
              echo "non-ff update blocked" >&2
              exit 1
            fi
          fi
        done
        unset IFS
      fi
      ;;
  esac

done
exit 0
`
	if err := os.WriteFile(preReceive, []byte(script), 0o755); err != nil {
		return err
	}
	return nil
}

func ensureRemoteRefExists(t *testing.T, remoteDir, ref string) {
	t.Helper()
	out, err := runCmdAllowFailure(t, remoteDir, nil, "git", "--git-dir", remoteDir, "show-ref", ref)
	if err != nil {
		t.Fatalf("expected remote ref %s, got error: %v\n%s", ref, err, out)
	}
}

func ensureRemoteRefMissing(t *testing.T, remoteDir, ref string) {
	t.Helper()
	out, err := runCmdAllowFailure(t, remoteDir, nil, "git", "--git-dir", remoteDir, "show-ref", ref)
	if err == nil {
		t.Fatalf("expected remote ref %s to be missing, got: %s", ref, out)
	}
}

func waitForProcessExit(cmd *exec.Cmd, timeout time.Duration) error {
	if cmd == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		if _, ok := err.(*exec.ExitError); ok {
			return nil
		}
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for process exit")
	}
}

func captureOutput(cmd *exec.Cmd) (*bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return stdout, stderr
}
