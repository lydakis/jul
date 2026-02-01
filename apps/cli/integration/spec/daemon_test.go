//go:build jul_integ_spec

package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to signal daemon: %v", err)
	}
	if err := waitForProcessExit(cmd, 2*time.Second); err != nil {
		t.Fatalf("daemon did not exit cleanly: %v", err)
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
