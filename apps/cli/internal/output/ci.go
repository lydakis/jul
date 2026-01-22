package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/lydakis/jul/cli/internal/ci"
)

type CIJSON struct {
	CI CIJSONDetails `json:"ci"`
}

type CIJSONDetails struct {
	Status     string    `json:"status"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	Results    []CICheck `json:"results,omitempty"`
}

type CICheck struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms,omitempty"`
	Output     string  `json:"output,omitempty"`
	Value      float64 `json:"value,omitempty"`
}

type CIStatusJSON struct {
	CI CIStatusDetails `json:"ci"`
}

type CIStatusDetails struct {
	Status          string    `json:"status"`
	CurrentDraftSHA string    `json:"current_draft_sha,omitempty"`
	CompletedSHA    string    `json:"completed_sha,omitempty"`
	ResultsCurrent  bool      `json:"results_current"`
	RunningSHA      string    `json:"running_sha,omitempty"`
	DurationMs      int64     `json:"duration_ms,omitempty"`
	Results         []CICheck `json:"results,omitempty"`
}

func RenderCIResult(out io.Writer, result ci.Result) {
	fmt.Fprintln(out, "Running CI...")
	for _, cmd := range result.Commands {
		icon := "✓"
		if cmd.Status != "pass" {
			icon = "✗"
		}
		fmt.Fprintf(out, "  %s %s (%dms)\n", icon, LabelForCommand(cmd.Command), cmd.DurationMs)
		if cmd.Status != "pass" && cmd.OutputExcerpt != "" {
			for _, line := range strings.Split(cmd.OutputExcerpt, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				fmt.Fprintf(out, "    %s\n", line)
			}
		}
	}
	if result.Status == "pass" {
		fmt.Fprintln(out, "All checks passed.")
		return
	}
	fmt.Fprintln(out, "One or more checks failed.")
}

func RenderCIStatus(out io.Writer, payload CIStatusJSON) {
	status := payload.CI
	fmt.Fprintln(out, "CI Status:")
	if status.CurrentDraftSHA != "" {
		fmt.Fprintf(out, "  Current draft: %s\n", status.CurrentDraftSHA)
	}
	if status.CompletedSHA != "" {
		marker := "✓"
		if !status.ResultsCurrent {
			marker = "⚠"
		}
		fmt.Fprintf(out, "  Last completed: %s %s\n", status.CompletedSHA, marker)
	}
	if status.RunningSHA != "" {
		fmt.Fprintf(out, "  ⚡ CI running for %s...\n", status.RunningSHA)
	}
	if len(status.Results) > 0 {
		fmt.Fprintln(out, "")
		for _, check := range status.Results {
			icon := "✓"
			if strings.ToLower(check.Status) != "pass" {
				icon = "✗"
			}
			if check.DurationMs > 0 {
				fmt.Fprintf(out, "  %s %s (%dms)\n", icon, check.Name, check.DurationMs)
			} else {
				fmt.Fprintf(out, "  %s %s\n", icon, check.Name)
			}
			if check.Output != "" && strings.ToLower(check.Status) != "pass" {
				for _, line := range strings.Split(check.Output, "\n") {
					if strings.TrimSpace(line) == "" {
						continue
					}
					fmt.Fprintf(out, "    %s\n", line)
				}
			}
		}
	}
}

func LabelForCommand(command string) string {
	normalized := strings.ToLower(strings.TrimSpace(command))
	switch {
	case strings.Contains(normalized, "lint"):
		return "lint"
	case strings.Contains(normalized, "go test") || strings.Contains(normalized, "pytest") ||
		strings.Contains(normalized, "npm test") || strings.Contains(normalized, "yarn test") ||
		strings.Contains(normalized, "pnpm test"):
		return "test"
	case strings.Contains(normalized, "go vet"):
		return "lint"
	default:
		if fields := strings.Fields(command); len(fields) > 0 {
			return fields[0]
		}
		return "command"
	}
}
