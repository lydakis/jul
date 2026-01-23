package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	cicmd "github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

func newCICommand() Command {
	return Command{
		Name:    "ci",
		Summary: "Run local CI and record attestation",
		Run: func(args []string) int {
			if len(args) == 0 {
				printCIUsage()
				return 1
			}
			if args[0] == "help" || args[0] == "--help" {
				printCIUsage()
				return 0
			}

			sub := args[0]
			switch sub {
			case "run":
				return runCIRun(args[1:])
			case "status":
				return runCIStatus(args[1:])
			case "list":
				return runCIList(args[1:])
			case "config":
				return runCIConfig(args[1:])
			case "cancel":
				return runCICancel(args[1:])
			default:
				printCIUsage()
				return 1
			}
		},
	}
}

func runCIRun(args []string) int {
	return runCIRunWithStream(args, nil, os.Stdout, os.Stderr, "", "manual")
}

func runCIRunWithStream(args []string, stream io.Writer, out io.Writer, errOut io.Writer, targetSHA string, mode string) int {
	fs := flag.NewFlagSet("ci run", flag.ContinueOnError)
	fs.SetOutput(out)
	var commands stringList
	fs.Var(&commands, "cmd", "Command to run (repeatable). Default: go test ./...")
	attType := fs.String("type", "ci", "Attestation type")
	target := fs.String("target", "", "Revision to attach results to (default: current draft)")
	commitSHA := fs.String("commit", "", "Alias for --target")
	changeIDFlag := fs.String("change", "", "Change-Id to attach results to (latest checkpoint)")
	coverageLine := fs.Float64("coverage-line", -1, "Coverage line percentage (optional)")
	coverageBranch := fs.Float64("coverage-branch", -1, "Coverage branch percentage (optional)")
	jsonOut := fs.Bool("json", false, "Output JSON")
	watch := fs.Bool("watch", false, "Stream output")
	_ = fs.Parse(args)

	cmds := []string(commands)

	if targetSHA == "" {
		targetSHA = strings.TrimSpace(*target)
	}
	if targetSHA == "" {
		targetSHA = strings.TrimSpace(*commitSHA)
	}
	if strings.TrimSpace(*changeIDFlag) != "" {
		if targetSHA != "" {
			fmt.Fprintln(errOut, "cannot combine --change with --target/--commit")
			return 1
		}
		resolved, err := resolveChangeTarget(*changeIDFlag)
		if err != nil {
			fmt.Fprintf(errOut, "failed to resolve change %s: %v\n", strings.TrimSpace(*changeIDFlag), err)
			return 1
		}
		targetSHA = resolved
	}
	info, err := resolveCICommit(targetSHA)
	if err != nil {
		fmt.Fprintf(errOut, "failed to read git state: %v\n", err)
		return 1
	}

	workdir := info.TopLevel
	if workdir == "" {
		if top, topErr := gitutil.RepoTopLevel(); topErr == nil {
			workdir = top
		}
	}
	if workdir == "" {
		fmt.Fprintln(errOut, "failed to determine repo root")
		return 1
	}

	if len(cmds) == 0 {
		if cfg, ok, err := cicmd.LoadConfig(); err == nil && ok && len(cfg.Commands) > 0 {
			for _, cmd := range cfg.Commands {
				if strings.TrimSpace(cmd.Command) == "" {
					continue
				}
				cmds = append(cmds, cmd.Command)
			}
		}
	}
	if len(cmds) == 0 {
		cmds = cicmd.InferDefaultCommands(workdir)
	}

	if *watch {
		stream = out
	}
	mode = resolveCIMode(mode)
	runID := strings.TrimSpace(os.Getenv("JUL_CI_RUN_ID"))
	if runID == "" {
		runID = cicmd.NewRunID()
	}
	logPath := strings.TrimSpace(os.Getenv("JUL_CI_LOG_PATH"))
	record := cicmd.RunRecord{
		ID:        runID,
		CommitSHA: info.SHA,
		Status:    "running",
		Mode:      mode,
		Commands:  cmds,
		StartedAt: time.Now().UTC(),
		LogPath:   logPath,
		PID:       os.Getpid(),
	}
	_ = cicmd.WriteRun(record)

	if mode == "draft" {
		_ = cicmd.WriteRunning(cicmd.Running{
			CommitSHA: info.SHA,
			StartedAt: time.Now().UTC(),
			PID:       os.Getpid(),
		})
		defer func() {
			_ = cicmd.ClearRunning()
		}()
	}

	var result cicmd.Result
	if stream != nil && !*jsonOut {
		fmt.Fprintln(out, "Running CI (streaming)...")
		result, err = cicmd.RunCommandsStreaming(cmds, workdir, stream)
	} else {
		result, err = cicmd.RunCommands(cmds, workdir)
	}
	if err != nil {
		record.Status = "error"
		record.FinishedAt = time.Now().UTC()
		_ = cicmd.WriteRun(record)
		fmt.Fprintf(errOut, "ci run failed: %v\n", err)
		return 1
	}

	changeID := info.ChangeID
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(info.SHA)
	}

	signals, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(errOut, "failed to encode signals: %v\n", err)
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

	created := client.Attestation{
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

	if !isDraftMessage(info.Message) {
		stored, err := metadata.WriteAttestation(created)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to record attestation: %v\n", err)
			return 1
		}
		created = stored
	}

	_ = cicmd.WriteCompleted(cicmd.Status{
		CommitSHA:         info.SHA,
		CompletedAt:       time.Now().UTC(),
		Result:            result,
		CoverageLinePct:   coverageLinePtr,
		CoverageBranchPct: coverageBranchPtr,
	})

	record.Status = result.Status
	record.FinishedAt = time.Now().UTC()
	_ = cicmd.WriteRun(record)

	if *jsonOut {
		payload := buildCIJSON(result, created)
		if err := renderCIJSON(payload); err != nil {
			fmt.Fprintf(errOut, "failed to encode json: %v\n", err)
			return 1
		}
		return exitCodeForStatus(result.Status)
	}

	output.RenderCIResult(out, result, output.DefaultOptions())
	return exitCodeForStatus(result.Status)
}

func printCIUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul ci run [--cmd <command>] [--watch] [--type ci] [--coverage-line <pct>] [--coverage-branch <pct>] [--target <rev>] [--change <id>] [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci status [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci list [--limit N] [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci config")
	fmt.Fprintln(os.Stdout, "       jul ci cancel")
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

	current, err := currentDraftSHA()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
		return 1
	}

	completed, err := cicmd.ReadCompleted()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read ci results: %v\n", err)
		return 1
	}
	running, _ := cicmd.ReadRunning()

	status := "unknown"
	resultsCurrent := false
	if completed != nil {
		resultsCurrent = completed.CommitSHA == current
		if resultsCurrent {
			status = completed.Result.Status
		} else {
			status = "stale"
		}
	}
	if running != nil && running.CommitSHA == current {
		status = "running"
	}

	payload := buildCIStatusJSON(status, current, completed, running, resultsCurrent)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		if status == "pass" {
			return 0
		}
		return 1
	}

	output.RenderCIStatus(os.Stdout, payload, output.DefaultOptions())
	if status == "pass" {
		return 0
	}
	return 1
}

func runCIConfig(args []string) int {
	fs := flag.NewFlagSet("ci config", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	initCfg := fs.Bool("init", false, "Create a default .jul/ci.toml if missing")
	showCfg := fs.Bool("show", false, "Show resolved commands")
	var sets stringList
	fs.Var(&sets, "set", "Add command (name=command)")
	_ = fs.Parse(args)

	if *showCfg {
		return showCIConfigResolved()
	}

	if *initCfg || len(sets) > 0 {
		if *initCfg && len(sets) == 0 {
			if path, err := cicmd.ConfigPath(); err == nil {
				if _, statErr := os.Stat(path); statErr == nil {
					fmt.Fprintln(os.Stdout, "CI configuration already exists.")
					return 0
				}
			}
		}
		commands := []cicmd.CommandSpec{}
		if len(sets) > 0 {
			for i, raw := range sets {
				name, cmd := splitCommandSpec(raw)
				if name == "" {
					name = fmt.Sprintf("cmd%d", i+1)
				}
				if strings.TrimSpace(cmd) == "" {
					fmt.Fprintf(os.Stderr, "invalid --set value: %s\n", raw)
					return 1
				}
				commands = append(commands, cicmd.CommandSpec{Name: name, Command: cmd})
			}
		} else {
			commands = []cicmd.CommandSpec{{Name: "test", Command: "go test ./..."}}
		}
		if err := cicmd.WriteConfig(commands); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write ci config: %v\n", err)
			return 1
		}
		fmt.Fprintln(os.Stdout, "CI configuration saved to .jul/ci.toml")
		return 0
	}

	fmt.Fprintln(os.Stdout, "CI configuration:")
	fmt.Fprintf(os.Stdout, "  run_on_checkpoint: %t\n", config.CIRunOnCheckpoint())
	fmt.Fprintf(os.Stdout, "  run_on_draft: %t\n", config.CIRunOnDraft())
	fmt.Fprintf(os.Stdout, "  draft_ci_blocking: %t\n", config.CIDraftBlocking())
	if cfg, ok, err := cicmd.LoadConfig(); err == nil && ok && len(cfg.Commands) > 0 {
		fmt.Fprintln(os.Stdout, "  commands (.jul/ci.toml):")
		for _, cmd := range cfg.Commands {
			if strings.TrimSpace(cmd.Command) == "" {
				continue
			}
			label := cmd.Command
			if strings.TrimSpace(cmd.Name) != "" {
				label = fmt.Sprintf("%s: %s", cmd.Name, cmd.Command)
			}
			fmt.Fprintf(os.Stdout, "    - %s\n", label)
		}
	} else {
		fmt.Fprintln(os.Stdout, "  default command: go test ./...")
	}
	return 0
}

func splitCommandSpec(raw string) (string, string) {
	if strings.Contains(raw, "=") {
		parts := strings.SplitN(raw, "=", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(raw)
}

func showCIConfigResolved() int {
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read repo root: %v\n", err)
		return 1
	}
	var cmds []string
	source := "inferred"
	if cfg, ok, err := cicmd.LoadConfig(); err == nil && ok && len(cfg.Commands) > 0 {
		for _, cmd := range cfg.Commands {
			if strings.TrimSpace(cmd.Command) == "" {
				continue
			}
			cmds = append(cmds, cmd.Command)
		}
		source = ".jul/ci.toml"
	} else {
		cmds = cicmd.InferDefaultCommands(root)
	}
	fmt.Fprintln(os.Stdout, "CI configuration (resolved):")
	fmt.Fprintf(os.Stdout, "  source: %s\n", source)
	fmt.Fprintln(os.Stdout, "  commands:")
	for _, cmd := range cmds {
		fmt.Fprintf(os.Stdout, "    - %s\n", cmd)
	}
	return 0
}

func runCICancel(args []string) int {
	_ = args
	running, err := cicmd.ReadRunning()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read running CI: %v\n", err)
		return 1
	}
	if running == nil || running.PID == 0 {
		fmt.Fprintln(os.Stdout, "No running CI job.")
		return 0
	}
	_ = cancelRunningCI(running)
	fmt.Fprintf(os.Stdout, "Cancelled CI run for %s\n", running.CommitSHA)
	return 0
}

func buildCIJSON(result cicmd.Result, att client.Attestation) output.CIJSON {
	details := output.CIJSONDetails{
		Status: result.Status,
	}
	if details.Status == "" {
		details.Status = att.Status
	}
	if !result.StartedAt.IsZero() && !result.FinishedAt.IsZero() {
		details.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	}
	if len(result.Commands) > 0 {
		checks := make([]output.CICheck, 0, len(result.Commands))
		for _, cmd := range result.Commands {
			checks = append(checks, output.CICheck{
				Name:       output.LabelForCommand(cmd.Command),
				Status:     cmd.Status,
				DurationMs: cmd.DurationMs,
				Output:     cmd.OutputExcerpt,
			})
		}
		if att.CoverageLinePct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_line",
				Status: "pass",
				Value:  *att.CoverageLinePct,
			})
		}
		if att.CoverageBranchPct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_branch",
				Status: "pass",
				Value:  *att.CoverageBranchPct,
			})
		}
		details.Results = checks
	}
	return output.CIJSON{CI: details}
}

func resolveCIMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = strings.TrimSpace(os.Getenv("JUL_CI_MODE"))
	}
	if mode == "" {
		mode = "manual"
	}
	return mode
}

func runCIList(args []string) int {
	fs := flag.NewFlagSet("ci list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	limit := fs.Int("limit", 10, "Max runs to show")
	jsonOut := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)

	runs, err := cicmd.ListRuns(*limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list ci runs: %v\n", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(output.CIRunsJSON{Runs: runs}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		return 0
	}
	output.RenderCIRuns(os.Stdout, runs, output.DefaultOptions())
	return 0
}

func renderCIJSON(payload output.CIJSON) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func exitCodeForStatus(status string) int {
	if status == "pass" {
		return 0
	}
	return 1
}

func resolveCICommit(targetSHA string) (gitutil.CommitInfo, error) {
	if strings.TrimSpace(targetSHA) == "" {
		sha, err := currentDraftSHA()
		if err != nil {
			return gitutil.CommitInfo{}, err
		}
		targetSHA = sha
	}
	sha, err := gitutil.Git("rev-parse", targetSHA)
	if err != nil {
		return gitutil.CommitInfo{}, err
	}
	msg, _ := gitutil.CommitMessage(sha)
	author, _ := gitutil.Git("log", "-1", "--format=%an", sha)
	top, _ := gitutil.RepoTopLevel()
	changeID := gitutil.ExtractChangeID(msg)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(sha)
	}
	return gitutil.CommitInfo{
		SHA:      strings.TrimSpace(sha),
		Author:   strings.TrimSpace(author),
		Message:  msg,
		ChangeID: changeID,
		TopLevel: strings.TrimSpace(top),
	}, nil
}

func resolveChangeTarget(changeID string) (string, error) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return "", nil
	}
	if checkpoint, err := latestCheckpointForChange(changeID); err != nil {
		return "", err
	} else if checkpoint != nil {
		return checkpoint.SHA, nil
	}
	draftSHA, err := currentDraftSHA()
	if err != nil {
		return "", fmt.Errorf("no checkpoint for change %s", changeID)
	}
	msg, _ := gitutil.CommitMessage(draftSHA)
	if gitutil.ExtractChangeID(msg) == changeID {
		return draftSHA, nil
	}
	return "", fmt.Errorf("no checkpoint for change %s", changeID)
}

func buildCIStatusJSON(status string, current string, completed *cicmd.Status, running *cicmd.Running, resultsCurrent bool) output.CIStatusJSON {
	details := output.CIStatusDetails{
		Status:          status,
		CurrentDraftSHA: current,
		ResultsCurrent:  resultsCurrent,
	}
	if completed != nil {
		details.CompletedSHA = completed.CommitSHA
		if !completed.Result.StartedAt.IsZero() && !completed.Result.FinishedAt.IsZero() {
			details.DurationMs = completed.Result.FinishedAt.Sub(completed.Result.StartedAt).Milliseconds()
		}
		checks := make([]output.CICheck, 0, len(completed.Result.Commands))
		for _, cmd := range completed.Result.Commands {
			checks = append(checks, output.CICheck{
				Name:       output.LabelForCommand(cmd.Command),
				Status:     cmd.Status,
				DurationMs: cmd.DurationMs,
				Output:     cmd.OutputExcerpt,
			})
		}
		if completed.CoverageLinePct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_line",
				Status: "pass",
				Value:  *completed.CoverageLinePct,
			})
		}
		if completed.CoverageBranchPct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_branch",
				Status: "pass",
				Value:  *completed.CoverageBranchPct,
			})
		}
		details.Results = checks
	}
	if running != nil {
		details.RunningSHA = running.CommitSHA
	}
	return output.CIStatusJSON{CI: details}
}
