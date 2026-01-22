package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	cicmd "github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func maybeRunDraftCI(res syncer.Result, jsonOut bool) int {
	if !config.CIRunOnDraft() || strings.TrimSpace(res.DraftSHA) == "" {
		return 0
	}
	if cfg, ok, err := cicmd.LoadConfig(); err != nil {
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "failed to read ci config: %v\n", err)
		}
		return 0
	} else if !ok || len(cfg.Commands) == 0 {
		return 0
	}

	if completed, err := cicmd.ReadCompleted(); err == nil && completed != nil {
		if strings.TrimSpace(completed.CommitSHA) == strings.TrimSpace(res.DraftSHA) {
			return 0
		}
	}
	if running, err := cicmd.ReadRunning(); err == nil && running != nil {
		if strings.TrimSpace(running.CommitSHA) == strings.TrimSpace(res.DraftSHA) {
			return 0
		}
		_ = cancelRunningCI(running)
	}

	if !config.CIDraftBlocking() {
		if err := startBackgroundCI(res.DraftSHA); err != nil {
			if !jsonOut {
				fmt.Fprintf(os.Stderr, "failed to start draft CI: %v\n", err)
			}
			return 0
		}
		if !jsonOut {
			fmt.Fprintln(os.Stdout, "  ⚡ CI running in background...")
		}
		return 0
	}

	out := io.Writer(os.Stdout)
	errOut := io.Writer(os.Stderr)
	if jsonOut {
		out = io.Discard
		errOut = io.Discard
	}
	return runCIRunWithStream([]string{}, nil, out, errOut, res.DraftSHA)
}

func cancelRunningCI(running *cicmd.Running) error {
	if running == nil || running.PID == 0 {
		return nil
	}
	proc, err := os.FindProcess(running.PID)
	if err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}
	return cicmd.ClearRunning()
}

func startBackgroundCI(targetSHA string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	logFile, err := openCILogFile(root)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "ci", "run", "--target", targetSHA)
	cmd.Dir = root
	cmd.Env = os.Environ()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	_ = logFile.Close()
	return nil
}

func openCILogFile(root string) (*os.File, error) {
	dir := filepath.Join(root, ".jul", "ci", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s.log", time.Now().UTC().Format("20060102-150405"))
	return os.Create(filepath.Join(dir, name))
}
