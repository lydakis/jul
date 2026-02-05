package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/syncer"
)

type syncRun struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	PID       int       `json:"pid"`
	LogPath   string    `json:"log_path,omitempty"`
}

type syncCompleted struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}

func startBackgroundSync(opts syncer.SyncOptions) error {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	if config.SyncMode() != "on-command" {
		return nil
	}
	debounce := time.Duration(config.SyncDebounceSeconds()) * time.Second
	minInterval := time.Duration(config.SyncMinIntervalSeconds()) * time.Second
	if debounce < 0 {
		debounce = 0
	}
	if minInterval < 0 {
		minInterval = 0
	}

	now := time.Now()
	if running, ok := readSyncRunning(repoRoot); ok {
		if syncRunActive(*running) {
			return nil
		}
		if debounce > 0 && !running.StartedAt.IsZero() && now.Sub(running.StartedAt) < debounce {
			return nil
		}
		_ = clearSyncRunning(repoRoot)
	}
	if completed, ok := readSyncCompleted(repoRoot); ok {
		if minInterval > 0 && !completed.FinishedAt.IsZero() && now.Sub(completed.FinishedAt) < minInterval {
			return nil
		}
		if debounce > 0 && !completed.FinishedAt.IsZero() && now.Sub(completed.FinishedAt) < debounce {
			return nil
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	runID := newRunID()
	logFile, logPath, err := openSyncLogFile(repoRoot, runID)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "sync", "--json")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"JUL_SYNC_RUN_ID="+runID,
		"JUL_SYNC_LOG_PATH="+logPath,
		"JUL_SYNC_BACKGROUND=1",
		"JUL_NO_SYNC=1",
	)
	if opts.AllowSecrets {
		cmd.Env = append(cmd.Env, "JUL_ALLOW_SECRETS=1")
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
		_ = cmd.Process.Release()
	}
	_ = logFile.Close()
	run := syncRun{
		ID:        runID,
		StartedAt: time.Now().UTC(),
		PID:       pid,
		LogPath:   logPath,
	}
	return writeSyncRunning(repoRoot, run)
}

func syncBackgroundEnv() string {
	return strings.TrimSpace(os.Getenv("JUL_SYNC_RUN_ID"))
}

func syncLogPathEnv() string {
	return strings.TrimSpace(os.Getenv("JUL_SYNC_LOG_PATH"))
}

func markSyncCompleted(repoRoot, runID string, err error) {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(runID) == "" {
		return
	}
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	_ = writeSyncCompleted(repoRoot, syncCompleted{
		ID:         runID,
		Status:     status,
		Error:      errMsg,
		FinishedAt: time.Now().UTC(),
	})
	_ = clearSyncRunning(repoRoot)
}

func readSyncRunning(repoRoot string) (*syncRun, bool) {
	path := syncRunningPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var run syncRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, false
	}
	return &run, true
}

func readSyncCompleted(repoRoot string) (*syncCompleted, bool) {
	path := syncCompletedPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var res syncCompleted
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, false
	}
	return &res, true
}

func writeSyncRunning(repoRoot string, run syncRun) error {
	path := syncRunningPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(run)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeSyncCompleted(repoRoot string, res syncCompleted) error {
	path := syncCompletedPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func clearSyncRunning(repoRoot string) error {
	path := syncRunningPath(repoRoot)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func syncRunningPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".jul", "sync", "running.json")
}

func syncCompletedPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".jul", "sync", "completed.json")
}

func openSyncLogFile(repoRoot, runID string) (*os.File, string, error) {
	dir := filepath.Join(repoRoot, ".jul", "sync", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	name := runID + ".log"
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		return nil, "", err
	}
	return file, path, nil
}

func newRunID() string {
	return time.Now().UTC().Format("20060102-150405.000000")
}
