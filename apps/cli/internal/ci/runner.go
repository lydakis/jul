package ci

import (
	"errors"
	"os/exec"
	"strings"
	"time"
)

const outputLimit = 4000

type CommandResult struct {
	Command       string `json:"command"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exit_code"`
	DurationMs    int64  `json:"duration_ms"`
	OutputExcerpt string `json:"output_excerpt,omitempty"`
}

type Result struct {
	Status     string          `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Commands   []CommandResult `json:"commands"`
}

func RunCommands(cmds []string, workdir string) (Result, error) {
	if len(cmds) == 0 {
		return Result{}, errors.New("no commands provided")
	}
	start := time.Now().UTC()
	results := make([]CommandResult, 0, len(cmds))
	overallStatus := "pass"

	for _, command := range cmds {
		cmdStart := time.Now()
		cmd := exec.Command("sh", "-c", command)
		if workdir != "" {
			cmd.Dir = workdir
		}
		output, err := cmd.CombinedOutput()
		code := 0
		status := "pass"
		if err != nil {
			status = "fail"
			code = exitCode(err)
			overallStatus = "fail"
		}

		results = append(results, CommandResult{
			Command:       command,
			Status:        status,
			ExitCode:      code,
			DurationMs:    time.Since(cmdStart).Milliseconds(),
			OutputExcerpt: truncate(string(output)),
		})

		if status == "fail" {
			break
		}
	}

	finished := time.Now().UTC()
	return Result{
		Status:     overallStatus,
		StartedAt:  start,
		FinishedAt: finished,
		Commands:   results,
	}, nil
}

func exitCode(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func truncate(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= outputLimit {
		return trimmed
	}
	return trimmed[:outputLimit]
}
