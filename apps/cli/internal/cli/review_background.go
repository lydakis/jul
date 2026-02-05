package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

type reviewRun struct {
	ID         string
	Mode       reviewMode
	StartedAt  time.Time
	PID        int
	LogPath    string
	ResultPath string
}

type reviewRunResult struct {
	Mode        reviewMode          `json:"mode"`
	ReviewID    string              `json:"review_id,omitempty"`
	Status      string              `json:"status,omitempty"`
	BaseSHA     string              `json:"base_sha,omitempty"`
	ChangeID    string              `json:"change_id,omitempty"`
	Summary     string              `json:"summary,omitempty"`
	Suggestions []client.Suggestion `json:"suggestions,omitempty"`
	Error       string              `json:"error,omitempty"`
	StartedAt   time.Time           `json:"started_at,omitempty"`
	FinishedAt  time.Time           `json:"finished_at,omitempty"`
}

func startBackgroundReview(mode reviewMode, fromReviewID string) (*reviewRun, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return nil, err
	}
	runID := newRunID()
	logFile, logPath, err := openReviewLogFile(root, runID)
	if err != nil {
		return nil, err
	}
	resultPath := reviewResultPath(root, runID)

	args := []string{"review"}
	if mode == reviewModeSuggest {
		args = append(args, "--suggest")
	}
	if strings.TrimSpace(fromReviewID) != "" {
		args = append(args, "--from", strings.TrimSpace(fromReviewID))
	}

	cmd := exec.Command(exe, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"JUL_REVIEW_INTERNAL=1",
		"JUL_REVIEW_RUN_ID="+runID,
		"JUL_REVIEW_RESULT_PATH="+resultPath,
		"JUL_REVIEW_BACKGROUND=1",
		"JUL_NO_SYNC=1",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
		_ = cmd.Process.Release()
	}
	_ = logFile.Close()
	return &reviewRun{
		ID:         runID,
		Mode:       mode,
		StartedAt:  time.Now().UTC(),
		PID:        pid,
		LogPath:    logPath,
		ResultPath: resultPath,
	}, nil
}

func reviewInternalEnv() bool {
	return strings.TrimSpace(os.Getenv("JUL_REVIEW_INTERNAL")) != ""
}

func reviewResultPathEnv() string {
	return strings.TrimSpace(os.Getenv("JUL_REVIEW_RESULT_PATH"))
}

func writeReviewResult(path string, res reviewRunResult) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readReviewResult(path string) (reviewRunResult, bool, error) {
	if strings.TrimSpace(path) == "" {
		return reviewRunResult{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return reviewRunResult{}, false, nil
		}
		return reviewRunResult{}, false, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return reviewRunResult{}, false, nil
	}
	var res reviewRunResult
	if err := json.Unmarshal(data, &res); err != nil {
		return reviewRunResult{}, false, nil
	}
	return res, true, nil
}

func waitForReviewResult(ctx context.Context, path string) (reviewRunResult, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		res, ok, err := readReviewResult(path)
		if err != nil {
			return reviewRunResult{}, err
		}
		if ok {
			return res, nil
		}
		select {
		case <-ctx.Done():
			return reviewRunResult{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func reviewResultPath(repoRoot, runID string) string {
	return filepath.Join(repoRoot, ".jul", "review", "results", runID+".json")
}

func openReviewLogFile(root, runID string) (*os.File, string, error) {
	dir := filepath.Join(root, ".jul", "review", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, runID+".log")
	file, err := os.Create(path)
	if err != nil {
		return nil, "", err
	}
	return file, path, nil
}
