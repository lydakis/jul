package ci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

type Status struct {
	CommitSHA         string    `json:"commit_sha"`
	CompletedAt       time.Time `json:"completed_at"`
	Result            Result    `json:"result"`
	CoverageLinePct   *float64  `json:"coverage_line_pct,omitempty"`
	CoverageBranchPct *float64  `json:"coverage_branch_pct,omitempty"`
}

type Running struct {
	CommitSHA string    `json:"commit_sha"`
	StartedAt time.Time `json:"started_at"`
	PID       int       `json:"pid"`
}

func WriteCompleted(status Status) error {
	if status.CommitSHA == "" {
		return fmt.Errorf("commit sha required")
	}
	if status.CompletedAt.IsZero() {
		status.CompletedAt = time.Now().UTC()
	}
	if err := writeTextFile("current_draft_sha", status.CommitSHA); err != nil {
		return err
	}
	path, err := ciPath("results.json")
	if err != nil {
		return err
	}
	return writeJSON(path, status)
}

func ReadCompleted() (*Status, error) {
	path, err := ciPath("results.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func WriteRunning(running Running) error {
	if running.CommitSHA == "" {
		return fmt.Errorf("commit sha required")
	}
	if running.StartedAt.IsZero() {
		running.StartedAt = time.Now().UTC()
	}
	if running.PID == 0 {
		return fmt.Errorf("pid required")
	}
	if err := writeTextFile("current_draft_sha", running.CommitSHA); err != nil {
		return err
	}
	return writeTextFile("current_run_pid", strconv.Itoa(running.PID))
}

func ReadRunning() (*Running, error) {
	pidStr, err := readTextFile("current_run_pid")
	if err != nil {
		return nil, err
	}
	if pidStr == "" {
		return nil, nil
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pid: %w", err)
	}
	commit, err := readTextFile("current_draft_sha")
	if err != nil {
		return nil, err
	}
	return &Running{
		CommitSHA: commit,
		PID:       pid,
	}, nil
}

func ClearRunning() error {
	path, err := ciPath("current_run_pid")
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ciPath(name string) (string, error) {
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, ".jul", "ci")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func writeTextFile(name, value string) error {
	path, err := ciPath(name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(value)), 0o644)
}

func readTextFile(name string) (string, error) {
	path, err := ciPath(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
