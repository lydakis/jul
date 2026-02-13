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
	RunningPID      int       `json:"running_pid,omitempty"`
	DurationMs      int64     `json:"duration_ms,omitempty"`
	Results         []CICheck `json:"results,omitempty"`
}

type CIRunsJSON struct {
	Runs []ci.RunRecord `json:"runs"`
}

func RenderCIResult(out io.Writer, result ci.Result, opts Options) {
	fmt.Fprintln(out, "Running CI...")
	for _, cmd := range result.Commands {
		icon := statusIconColored(cmd.Status, opts)
		if icon == "" {
			if strings.ToLower(cmd.Status) == "pass" {
				icon = statusIcon("pass", opts)
			} else {
				icon = statusIcon("fail", opts)
			}
		}
		fmt.Fprintf(out, "  %s%s (%dms)\n", icon, LabelForCommand(cmd.Command), cmd.DurationMs)
		if strings.ToLower(cmd.Status) != "pass" && cmd.OutputExcerpt != "" {
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

func RenderCIJSON(out io.Writer, payload CIJSON, opts Options) {
	details := payload.CI
	fmt.Fprintln(out, "Running CI...")
	for _, check := range details.Results {
		icon := statusIconColored(check.Status, opts)
		if icon == "" {
			if strings.ToLower(check.Status) == "pass" {
				icon = statusIcon("pass", opts)
			} else {
				icon = statusIcon("fail", opts)
			}
		}
		label := check.Name
		if check.Value != 0 {
			label = fmt.Sprintf("%s %g", check.Name, check.Value)
		}
		if check.DurationMs > 0 {
			fmt.Fprintf(out, "  %s%s (%dms)\n", icon, label, check.DurationMs)
		} else {
			fmt.Fprintf(out, "  %s%s\n", icon, label)
		}
		if strings.ToLower(check.Status) != "pass" && check.Output != "" {
			for _, line := range strings.Split(check.Output, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				fmt.Fprintf(out, "    %s\n", line)
			}
		}
	}
	if details.Status == "pass" {
		fmt.Fprintln(out, "All checks passed.")
		return
	}
	fmt.Fprintln(out, "One or more checks failed.")
}

func RenderCIStatus(out io.Writer, payload CIStatusJSON, opts Options) {
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
		icon := statusIconColored("running", opts)
		if icon == "" {
			if opts.Emoji {
				icon = "⚡ "
			} else {
				icon = "* "
			}
		}
		if status.RunningPID > 0 {
			fmt.Fprintf(out, "  %sCI running for %s (pid %d)...\n", icon, status.RunningSHA, status.RunningPID)
		} else {
			fmt.Fprintf(out, "  %sCI running for %s...\n", icon, status.RunningSHA)
		}
	}
	if len(status.Results) > 0 {
		fmt.Fprintln(out, "")
		for _, check := range status.Results {
			icon := statusIconColored(check.Status, opts)
			if icon == "" {
				if strings.ToLower(check.Status) == "pass" {
					icon = statusIcon("pass", opts)
				} else {
					icon = statusIcon("fail", opts)
				}
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

func RenderCIRuns(out io.Writer, runs []ci.RunRecord, opts Options) {
	if len(runs) == 0 {
		fmt.Fprintln(out, "No CI runs recorded.")
		return
	}
	fmt.Fprintln(out, "CI Runs:")
	for _, run := range runs {
		icon := statusIconColored(run.Status, opts)
		if icon == "" {
			icon = statusIcon(run.Status, opts)
		}
		start := ""
		if !run.StartedAt.IsZero() {
			start = run.StartedAt.Format("2006-01-02 15:04:05")
		}
		mode := run.Mode
		if mode == "" {
			mode = "manual"
		}
		sha := shortID(run.CommitSHA, 6)
		line := fmt.Sprintf("  %s%s %s %s %s", icon, start, mode, sha, run.ID)
		fmt.Fprintln(out, strings.TrimSpace(line))
	}
}

func LabelForCommand(command string) string {
	normalized := strings.ToLower(strings.TrimSpace(command))
	if base, path := splitChdirCommand(normalized, command); base != "" && path != "" {
		return fmt.Sprintf("%s (%s)", base, path)
	}
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

func splitChdirCommand(normalized, original string) (string, string) {
	trimmed := strings.TrimSpace(original)
	if !strings.HasPrefix(strings.ToLower(trimmed), "cd ") {
		return "", ""
	}
	rest := strings.TrimSpace(trimmed[3:])
	sep := "&&"
	idx := strings.Index(rest, sep)
	if idx < 0 {
		sep = ";"
		idx = strings.Index(rest, sep)
	}
	if idx < 0 {
		return "", ""
	}
	path := strings.TrimSpace(rest[:idx])
	path = strings.Trim(path, "\"'")
	cmd := strings.TrimSpace(rest[idx+len(sep):])
	if path == "" || cmd == "" {
		return "", ""
	}
	base := LabelForCommand(cmd)
	if base == "" {
		return "", ""
	}
	return base, path
}
