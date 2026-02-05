package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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
		if _, err := startBackgroundCI(res.DraftSHA, "draft"); err != nil {
			if !jsonOut {
				fmt.Fprintf(os.Stderr, "failed to start draft CI: %v\n", err)
			}
			return 0
		}
		if !jsonOut {
			fmt.Fprintln(os.Stdout, "  ⚡ CI running in background... (jul ci status)")
		}
		return 0
	}

	out := io.Writer(os.Stdout)
	errOut := io.Writer(os.Stderr)
	if jsonOut {
		out = io.Discard
		errOut = io.Discard
	}
	if !jsonOut {
		fmt.Fprintln(os.Stdout, "  ⚡ CI triggered by sync")
	}
	return runCIRunWithStream([]string{}, nil, out, errOut, res.DraftSHA, "draft")
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

type ciRun struct {
	ID      string
	LogPath string
}

func startBackgroundCI(targetSHA, mode string) (*ciRun, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return nil, err
	}
	runID := cicmd.NewRunID()
	logFile, logPath, err := openCILogFile(root, runID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(mode) == "" {
		mode = "draft"
	}
	cmd := exec.Command(exe, "ci", "run", "--target", targetSHA)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"JUL_CI_MODE="+mode,
		"JUL_CI_RUN_ID="+runID,
		"JUL_CI_LOG_PATH="+logPath,
		"JUL_CI_BACKGROUND=1",
		"JUL_NO_SYNC=1",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	_ = logFile.Close()
	return &ciRun{ID: runID, LogPath: logPath}, nil
}

func openCILogFile(root string, runID string) (*os.File, string, error) {
	dir := filepath.Join(root, ".jul", "ci", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	name := fmt.Sprintf("%s.log", runID)
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		return nil, "", err
	}
	return file, path, nil
}
