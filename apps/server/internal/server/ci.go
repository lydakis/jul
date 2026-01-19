package server

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const ciOutputLimit = 4000

type ciCommandResult struct {
	Command       string `json:"command"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exit_code"`
	DurationMs    int64  `json:"duration_ms"`
	OutputExcerpt string `json:"output_excerpt,omitempty"`
}

type ciResult struct {
	Status     string            `json:"status"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	Commands   []ciCommandResult `json:"commands"`
}

func runCI(repoPath, commitSHA string, commands []string) (ciResult, error) {
	start := time.Now().UTC()
	worktreeDir, err := os.MkdirTemp("", "jul-ci-*")
	if err != nil {
		return ciResult{}, err
	}

	cleanup := func() {
		_ = exec.Command("git", "--git-dir", repoPath, "worktree", "remove", "--force", worktreeDir).Run()
		_ = os.RemoveAll(worktreeDir)
	}
	defer cleanup()

	add := exec.Command("git", "--git-dir", repoPath, "worktree", "add", "--detach", worktreeDir, commitSHA)
	if output, err := add.CombinedOutput(); err != nil {
		return ciResult{}, fmtError("git worktree add failed", output, err)
	}

	results := make([]ciCommandResult, 0, len(commands))
	status := "pass"

	for _, command := range commands {
		cmdStart := time.Now()
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = worktreeDir
		output, err := cmd.CombinedOutput()
		code := 0
		cmdStatus := "pass"
		if err != nil {
			cmdStatus = "fail"
			code = exitCode(err)
			status = "fail"
		}

		results = append(results, ciCommandResult{
			Command:       command,
			Status:        cmdStatus,
			ExitCode:      code,
			DurationMs:    time.Since(cmdStart).Milliseconds(),
			OutputExcerpt: truncateOutput(string(output)),
		})

		if cmdStatus == "fail" {
			break
		}
	}

	return ciResult{
		Status:     status,
		StartedAt:  start,
		FinishedAt: time.Now().UTC(),
		Commands:   results,
	}, nil
}

func truncateOutput(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= ciOutputLimit {
		return trimmed
	}
	return trimmed[:ciOutputLimit]
}

func fmtError(prefix string, output []byte, err error) error {
	msg := strings.TrimSpace(string(output))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%s: %s", prefix, msg)
}

func exitCode(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}
