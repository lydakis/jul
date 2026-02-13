//go:build jul_integ_spec

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestIT_DAEMON_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	cmd1 := exec.Command(julPath, "sync", "--daemon")
	cmd1.Dir = repo
	cmd1.Env = mergeEnv(device.Env)
	stdout1, stderr1 := captureOutput(cmd1)
	if err := cmd1.Start(); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	defer func() {
		_ = cmd1.Process.Signal(syscall.SIGTERM)
		_ = cmd1.Wait()
	}()

	waitForOutput(t, stdout1, stderr1, "Sync daemon running")

	pidPath := filepath.Join(repo, ".jul", "sync-daemon.pid")
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("failed to read daemon pid: %v", err)
	}
	pidVal, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		t.Fatalf("invalid daemon pid %q: %v", string(pidData), err)
	}
	if cmd1.Process != nil && pidVal != cmd1.Process.Pid {
		t.Fatalf("expected pid file %d, got %d", cmd1.Process.Pid, pidVal)
	}

	cmd2 := exec.Command(julPath, "sync", "--daemon")
	cmd2.Dir = repo
	cmd2.Env = mergeEnv(device.Env)
	stdout2, stderr2 := captureOutput(cmd2)
	if err := cmd2.Run(); err != nil {
		_ = err
	}
	combined := stdout2.String() + stderr2.String()
	if !strings.Contains(combined, "already running") {
		t.Fatalf("expected second daemon start to report already running, got %s", combined)
	}
	pidData2, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("failed to read daemon pid after second start: %v", err)
	}
	if strings.TrimSpace(string(pidData2)) != strings.TrimSpace(string(pidData)) {
		t.Fatalf("expected pid file to remain unchanged, got %s vs %s", strings.TrimSpace(string(pidData)), strings.TrimSpace(string(pidData2)))
	}

	statOut, err := exec.Command("ps", "-p", strconv.Itoa(pidVal), "-o", "stat=").Output()
	if err != nil {
		t.Fatalf("failed to read daemon process state: %v", err)
	}
	if strings.Contains(strings.ToUpper(strings.TrimSpace(string(statOut))), "Z") {
		t.Fatalf("expected daemon not to be a zombie, got %s", strings.TrimSpace(string(statOut)))
	}
}

func TestIT_DAEMON_002(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	cmd := exec.Command(julPath, "sync", "--daemon")
	cmd.Dir = repo
	cmd.Env = mergeEnv(device.Env)
	stdout, stderr := captureOutput(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	waitForOutput(t, stdout, stderr, "Sync daemon running")

	draftIndexPath := filepath.Join(repo, ".jul", "draft-index")
	waitForFile(t, draftIndexPath, 2*time.Second)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to signal daemon: %v", err)
	}
	if err := waitForProcessExit(cmd, 2*time.Second); err != nil {
		t.Fatalf("daemon did not exit cleanly: %v", err)
	}

	pidPath := filepath.Join(repo, ".jul", "sync-daemon.pid")
	if _, err := os.Stat(pidPath); err == nil {
		t.Fatalf("expected pid file to be removed on shutdown")
	}

	lockPath := filepath.Join(repo, ".jul", "draft-index.lock")
	if _, err := os.Stat(lockPath); err == nil {
		t.Fatalf("expected draft index lock to be removed on shutdown")
	}

	childrenOut, err := exec.Command("ps", "-o", "pid=", "--ppid", strconv.Itoa(cmd.Process.Pid)).Output()
	if err == nil && strings.TrimSpace(string(childrenOut)) != "" {
		t.Fatalf("expected no child processes after shutdown, got %s", strings.TrimSpace(string(childrenOut)))
	}

	syncOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var syncRes syncResult
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&syncRes); err != nil {
		t.Fatalf("failed to decode sync output after daemon shutdown: %v", err)
	}
	if syncRes.DraftSHA == "" {
		t.Fatalf("expected sync to succeed after daemon shutdown, got %s", syncOut)
	}
}

func TestIT_DAEMON_003(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	pidPath := filepath.Join(repo, ".jul", "sync-daemon.pid")
	if err := os.WriteFile(pidPath, []byte("999999\n"), 0o644); err != nil {
		t.Fatalf("failed to seed stale daemon pid marker: %v", err)
	}

	cmd := exec.Command(julPath, "sync", "--daemon")
	cmd.Dir = repo
	cmd.Env = mergeEnv(device.Env)
	stdout, stderr := captureOutput(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start daemon with stale pid marker: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	}()

	waitForOutput(t, stdout, stderr, "Sync daemon running")
	if strings.Contains(stdout.String()+stderr.String(), "already running") {
		t.Fatalf("expected stale pid marker recovery to start daemon, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("failed to read daemon pid marker after recovery: %v", err)
	}
	pidVal, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		t.Fatalf("invalid daemon pid marker %q: %v", string(pidData), err)
	}
	if cmd.Process == nil || pidVal != cmd.Process.Pid {
		t.Fatalf("expected daemon pid marker to be refreshed to %d, got %d", cmd.Process.Pid, pidVal)
	}
}

func TestIT_DAEMON_009(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	writeFile(t, repo, ".jul/config.toml", "[sync]\ndebounce_seconds = 1\nmin_interval_seconds = 1\n")

	cmd := exec.Command(julPath, "sync", "--daemon")
	cmd.Dir = repo
	cmd.Env = mergeEnv(device.Env)
	stdout, stderr := captureOutput(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	}()

	waitForOutput(t, stdout, stderr, "Sync daemon running")
	waitForOutput(t, stdout, stderr, "\"event\":\"daemon_sync_done\"")
	time.Sleep(200 * time.Millisecond)

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	beforeSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", syncRef))
	beforeStarts := strings.Count(stdout.String()+"\n"+stderr.String(), "\"event\":\"daemon_sync_start\"")
	if beforeStarts < 1 {
		t.Fatalf("expected at least one daemon sync start, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	agentWorktree := filepath.Join(repo, ".jul", "agent-workspace", "worktree")
	if err := os.MkdirAll(agentWorktree, 0o755); err != nil {
		t.Fatalf("failed to create .jul/agent-workspace/worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentWorktree, "scratch.txt"), []byte("touch-1\n"), 0o644); err != nil {
		t.Fatalf("failed to write .jul agent-workspace scratch file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentWorktree, "scratch-2.txt"), []byte("touch-2\n"), 0o644); err != nil {
		t.Fatalf("failed to write .jul agent-workspace scratch file: %v", err)
	}
	time.Sleep(2500 * time.Millisecond)

	afterStarts := strings.Count(stdout.String()+"\n"+stderr.String(), "\"event\":\"daemon_sync_start\"")
	if afterStarts != beforeStarts {
		t.Fatalf("expected .jul/agent-workspace/worktree churn to be ignored by daemon sync (starts %d -> %d), stdout=%q stderr=%q", beforeStarts, afterStarts, stdout.String(), stderr.String())
	}
	afterSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", syncRef))
	if afterSHA != beforeSHA {
		t.Fatalf("expected sync ref unchanged after .jul/agent-workspace/worktree churn, got %s -> %s", beforeSHA, afterSHA)
	}
}

func TestIT_ROBUST_005(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	cmd := exec.Command(julPath, "sync", "--daemon")
	cmd.Dir = repo
	cmd.Env = mergeEnv(device.Env)
	stdout, stderr := captureOutput(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	}()
	waitForOutput(t, stdout, stderr, "Sync daemon running")

	julDir := filepath.Join(repo, ".jul")
	if err := os.RemoveAll(julDir); err != nil {
		t.Fatalf("failed to remove .jul: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		// Daemon exited; acceptable.
		return
	}
	time.Sleep(300 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("expected daemon to remain running after .jul deletion, got %v", err)
	}

	if _, err := os.Stat(julDir); err != nil {
		t.Fatalf("expected daemon to recreate or tolerate .jul removal, got %v", err)
	}
}

func waitForOutput(t *testing.T, stdout, stderr *bytes.Buffer, needle string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			combined := stdout.String() + stderr.String()
			t.Fatalf("timeout waiting for output %q, got %s", needle, combined)
		default:
			combined := stdout.String() + stderr.String()
			if strings.Contains(combined, needle) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for file %s", path)
		default:
			if _, err := os.Stat(path); err == nil {
				return
			} else if !os.IsNotExist(err) {
				t.Fatalf("failed to stat %s: %v", path, err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
