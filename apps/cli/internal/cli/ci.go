package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	cicmd "github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

func newCICommand() Command {
	return Command{
		Name:    "ci",
		Summary: "Run local CI and record attestation",
		Run: func(args []string) int {
			if len(args) == 0 {
				return runCIRun(args)
			}
			if args[0] == "help" || args[0] == "--help" {
				printCIUsage()
				return 0
			}
			if strings.HasPrefix(args[0], "-") {
				return runCIRun(args)
			}

			sub := args[0]
			switch sub {
			case "run":
				return runCIRun(args[1:])
			case "status":
				return runCIStatus(args[1:])
			case "watch":
				return runCIRun(args[1:])
			case "config":
				return runCIConfig(args[1:])
			default:
				printCIUsage()
				return 1
			}
		},
	}
}

func runCIRun(args []string) int {
	fs := flag.NewFlagSet("ci run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var commands stringList
	fs.Var(&commands, "cmd", "Command to run (repeatable). Default: go test ./...")
	attType := fs.String("type", "ci", "Attestation type")
	coverageLine := fs.Float64("coverage-line", -1, "Coverage line percentage (optional)")
	coverageBranch := fs.Float64("coverage-branch", -1, "Coverage branch percentage (optional)")
	jsonOut := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)

	cmds := []string(commands)
	if len(cmds) == 0 {
		cmds = []string{"go test ./..."}
	}

	info, err := gitutil.CurrentCommit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
		return 1
	}

	workdir := info.TopLevel
	if workdir == "" {
		if top, err := gitutil.RepoTopLevel(); err == nil {
			workdir = top
		}
	}
	if workdir == "" {
		fmt.Fprintln(os.Stderr, "failed to determine repo root")
		return 1
	}

	result, err := cicmd.RunCommands(cmds, workdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ci run failed: %v\n", err)
		return 1
	}

	changeID := info.ChangeID
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(info.SHA)
	}

	signals, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode signals: %v\n", err)
		return 1
	}

	testStatus, compileStatus := inferCIStatuses(cmds, result.Status)
	var coverageLinePtr *float64
	if *coverageLine >= 0 {
		coverageLinePtr = coverageLine
	}
	var coverageBranchPtr *float64
	if *coverageBranch >= 0 {
		coverageBranchPtr = coverageBranch
	}

	att := client.Attestation{
		CommitSHA:         info.SHA,
		ChangeID:          changeID,
		Type:              *attType,
		Status:            result.Status,
		TestStatus:        testStatus,
		CompileStatus:     compileStatus,
		CoverageLinePct:   coverageLinePtr,
		CoverageBranchPct: coverageBranchPtr,
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
		SignalsJSON:       string(signals),
	}

	created, err := metadata.WriteAttestation(att)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to record attestation: %v\n", err)
		return 1
	}

	if *jsonOut {
		payload := buildCIJSON(result, created)
		if err := renderCIJSON(payload); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		return exitCodeForStatus(result.Status)
	}

	renderCIHuman(result)
	return exitCodeForStatus(result.Status)
}

func printCIUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul ci [run] [--cmd <command>] [--type ci] [--coverage-line <pct>] [--coverage-branch <pct>] [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci status [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci watch [--cmd <command>]")
	fmt.Fprintln(os.Stdout, "       jul ci config")
}

func inferCIStatuses(commands []string, overall string) (string, string) {
	testStatus := ""
	compileStatus := ""
	for _, cmd := range commands {
		normalized := strings.ToLower(strings.TrimSpace(cmd))
		if strings.Contains(normalized, "go test") || strings.Contains(normalized, "pytest") ||
			strings.Contains(normalized, "npm test") || strings.Contains(normalized, "yarn test") ||
			strings.Contains(normalized, "pnpm test") {
			testStatus = overall
			compileStatus = overall
		}
		if strings.Contains(normalized, "go build") || strings.Contains(normalized, "go test") {
			if compileStatus == "" {
				compileStatus = overall
			}
		}
	}
	return testStatus, compileStatus
}

func runCIStatus(args []string) int {
	fs := flag.NewFlagSet("ci status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	jsonOut := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)

	info, err := gitutil.CurrentCommit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
		return 1
	}

	att, err := metadata.GetAttestation(info.SHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read attestation: %v\n", err)
		return 1
	}
	if att == nil {
		if *jsonOut {
			_ = renderCIJSON(ciJSON{CI: ciJSONDetails{Status: "unknown"}})
			return 1
		}
		fmt.Fprintln(os.Stdout, "No CI results for current commit.")
		return 1
	}

	var result cicmd.Result
	if strings.TrimSpace(att.SignalsJSON) != "" {
		_ = json.Unmarshal([]byte(att.SignalsJSON), &result)
	}

	if *jsonOut {
		payload := buildCIJSON(result, *att)
		if err := renderCIJSON(payload); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		return exitCodeForStatus(att.Status)
	}

	if result.Status == "" {
		fmt.Fprintf(os.Stdout, "ci %s (commit %s)\n", att.Status, info.SHA)
		return exitCodeForStatus(att.Status)
	}
	renderCIHuman(result)
	return exitCodeForStatus(result.Status)
}

func runCIConfig(args []string) int {
	_ = args
	fmt.Fprintln(os.Stdout, "CI configuration:")
	fmt.Fprintln(os.Stdout, "  run_on_checkpoint: true (default)")
	fmt.Fprintln(os.Stdout, "  run_on_draft: true (default)")
	fmt.Fprintln(os.Stdout, "  draft_ci_blocking: false (default)")
	fmt.Fprintln(os.Stdout, "  default command: go test ./...")
	return 0
}

type ciJSON struct {
	CI ciJSONDetails `json:"ci"`
}

type ciJSONDetails struct {
	Status     string        `json:"status"`
	DurationMs int64         `json:"duration_ms,omitempty"`
	Results    []ciJSONCheck `json:"results,omitempty"`
}

type ciJSONCheck struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms,omitempty"`
	Output     string  `json:"output,omitempty"`
	Value      float64 `json:"value,omitempty"`
}

func buildCIJSON(result cicmd.Result, att client.Attestation) ciJSON {
	details := ciJSONDetails{
		Status: result.Status,
	}
	if details.Status == "" {
		details.Status = att.Status
	}
	if !result.StartedAt.IsZero() && !result.FinishedAt.IsZero() {
		details.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	}
	if len(result.Commands) > 0 {
		checks := make([]ciJSONCheck, 0, len(result.Commands))
		for _, cmd := range result.Commands {
			checks = append(checks, ciJSONCheck{
				Name:       labelForCommand(cmd.Command),
				Status:     cmd.Status,
				DurationMs: cmd.DurationMs,
				Output:     cmd.OutputExcerpt,
			})
		}
		if att.CoverageLinePct != nil {
			status := "pass"
			checks = append(checks, ciJSONCheck{
				Name:   "coverage",
				Status: status,
				Value:  *att.CoverageLinePct,
			})
		}
		details.Results = checks
	}
	return ciJSON{CI: details}
}

func renderCIJSON(payload ciJSON) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func renderCIHuman(result cicmd.Result) {
	fmt.Fprintln(os.Stdout, "Running CI...")
	for _, cmd := range result.Commands {
		icon := "✓"
		if cmd.Status != "pass" {
			icon = "✗"
		}
		fmt.Fprintf(os.Stdout, "  %s %s (%dms)\n", icon, labelForCommand(cmd.Command), cmd.DurationMs)
		if cmd.Status != "pass" && cmd.OutputExcerpt != "" {
			for _, line := range strings.Split(cmd.OutputExcerpt, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				fmt.Fprintf(os.Stdout, "    %s\n", line)
			}
		}
	}
	if result.Status == "pass" {
		fmt.Fprintln(os.Stdout, "All checks passed.")
		return
	}
	fmt.Fprintln(os.Stdout, "One or more checks failed.")
}

func labelForCommand(command string) string {
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

func exitCodeForStatus(status string) int {
	if status == "pass" {
		return 0
	}
	return 1
}
