package cli

import (
	"encoding/json"
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
			jsonOut, args := stripJSONFlag(args)
			if len(args) == 0 {
				if jsonOut {
					_ = output.EncodeError(os.Stdout, "ci_missing_subcommand", "missing ci subcommand", nil)
					return 1
				}
				printCIUsage()
				return 1
			}
			if args[0] == "help" || args[0] == "--help" {
				printCIUsage()
				return 0
			}

			sub := args[0]
			subArgs := args[1:]
			if jsonOut {
				subArgs = ensureJSONFlag(subArgs)
			}
			switch sub {
			case "run":
				return runCIRun(subArgs)
			case "status":
				return runCIStatus(subArgs)
			case "list":
				return runCIList(subArgs)
			case "config":
				return runCIConfig(subArgs)
			case "cancel":
				return runCICancel(subArgs)
			default:
				if jsonOut {
					_ = output.EncodeError(os.Stdout, "ci_unknown_subcommand", fmt.Sprintf("unknown subcommand %q", sub), nil)
					return 1
				}
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
	fs, jsonOut := newFlagSetWithOutput("ci run", out)
	var commands stringList
	fs.Var(&commands, "cmd", "Command to run (repeatable). Default: go test ./...")
	attType := fs.String("type", "ci", "Attestation type")
	target := fs.String("target", "", "Revision to attach results to (default: current draft)")
	commitSHA := fs.String("commit", "", "Alias for --target")
	changeIDFlag := fs.String("change", "", "Change-Id to attach results to (latest checkpoint)")
	coverageLine := fs.Float64("coverage-line", -1, "Coverage line percentage (optional)")
	coverageBranch := fs.Float64("coverage-branch", -1, "Coverage branch percentage (optional)")
	watch := fs.Bool("watch", false, "Stream output")
	_ = fs.Parse(args)
	if !*watch && watchEnabled() {
		*watch = true
	}

	cmds := []string(commands)
	writeErr := func(code, msg string) int {
		if *jsonOut {
			_ = output.EncodeError(out, code, msg, nil)
		} else {
			fmt.Fprintln(errOut, msg)
		}
		return 1
	}

	if targetSHA == "" {
		targetSHA = strings.TrimSpace(*target)
	}
	if targetSHA == "" {
		targetSHA = strings.TrimSpace(*commitSHA)
	}
	if strings.TrimSpace(*changeIDFlag) != "" {
		if targetSHA != "" {
			return writeErr("ci_invalid_args", "cannot combine --change with --target/--commit")
		}
		resolved, err := resolveChangeTarget(*changeIDFlag)
		if err != nil {
			return writeErr("ci_change_resolve_failed", fmt.Sprintf("failed to resolve change %s: %v", strings.TrimSpace(*changeIDFlag), err))
		}
		targetSHA = resolved
	}
	info, err := resolveCICommit(targetSHA)
	if err != nil {
		return writeErr("ci_git_state_failed", fmt.Sprintf("failed to read git state: %v", err))
	}

	workdir := info.TopLevel
	if workdir == "" {
		if top, topErr := gitutil.RepoTopLevel(); topErr == nil {
			workdir = top
		}
	}
	if workdir == "" {
		return writeErr("ci_repo_root_failed", "failed to determine repo root")
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
		return writeErr("ci_run_failed", fmt.Sprintf("ci run failed: %v", err))
	}

	changeID := info.ChangeID
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(info.SHA)
	}

	signals, err := json.Marshal(result)
	if err != nil {
		return writeErr("ci_signals_failed", fmt.Sprintf("failed to encode signals: %v", err))
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
	deviceID, _ := config.DeviceID()

	created := client.Attestation{
		CommitSHA:         info.SHA,
		DeviceID:          strings.TrimSpace(deviceID),
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
			return writeErr("ci_attestation_failed", fmt.Sprintf("failed to record attestation: %v", err))
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

	payload := buildCIJSON(result, created)
	if *jsonOut {
		if code := writeJSONTo(out, payload); code != 0 {
			return code
		}
		return exitCodeForStatus(result.Status)
	}

	output.RenderCIJSON(out, payload, output.DefaultOptions())
	return exitCodeForStatus(result.Status)
}

func printCIUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul ci run [--cmd <command>] [--watch] [--type ci] [--coverage-line <pct>] [--coverage-branch <pct>] [--target <rev>] [--change <id>] [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci status [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci list [--limit N] [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci config [--init] [--set name=cmd] [--show] [--json]")
	fmt.Fprintln(os.Stdout, "       jul ci cancel [--json]")
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
	fs, jsonOut := newFlagSet("ci status")
	_ = fs.Parse(args)

	current, err := currentDraftSHA()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "ci_status_failed", fmt.Sprintf("failed to read git state: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
		}
		return 1
	}

	completed, err := cicmd.ReadCompleted()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "ci_status_failed", fmt.Sprintf("failed to read ci results: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to read ci results: %v\n", err)
		}
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
		if code := writeJSON(payload); code != 0 {
			return code
		}
		return exitCodeForStatus(status)
	}

	output.RenderCIStatus(os.Stdout, payload, output.DefaultOptions())
	return exitCodeForStatus(status)
}

func runCIConfig(args []string) int {
	fs, jsonOut := newFlagSet("ci config")
	initCfg := fs.Bool("init", false, "Create a default .jul/ci.toml if missing")
	showCfg := fs.Bool("show", false, "Show resolved commands")
	var sets stringList
	fs.Var(&sets, "set", "Add command (name=command)")
	_ = fs.Parse(args)

	if *showCfg {
		out, err := buildCIConfigResolvedOutput()
		if err != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "ci_config_failed", fmt.Sprintf("failed to read repo root: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "failed to read repo root: %v\n", err)
			}
			return 1
		}
		return writeCIConfigOutput(out, *jsonOut)
	}

	if *initCfg || len(sets) > 0 {
		if *initCfg && len(sets) == 0 {
			if path, err := cicmd.ConfigPath(); err == nil {
				if _, statErr := os.Stat(path); statErr == nil {
					out := ciConfigOutput{Status: "ok", Message: "CI configuration already exists."}
					return writeCIConfigOutput(out, *jsonOut)
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
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "ci_config_invalid_set", fmt.Sprintf("invalid --set value: %s", raw), nil)
					} else {
						fmt.Fprintf(os.Stderr, "invalid --set value: %s\n", raw)
					}
					return 1
				}
				commands = append(commands, cicmd.CommandSpec{Name: name, Command: cmd})
			}
		} else {
			commands = []cicmd.CommandSpec{{Name: "test", Command: "go test ./..."}}
		}
		if err := cicmd.WriteConfig(commands); err != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "ci_config_write_failed", fmt.Sprintf("failed to write ci config: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "failed to write ci config: %v\n", err)
			}
			return 1
		}
		out := ciConfigOutput{Status: "ok", Message: "CI configuration saved to .jul/ci.toml"}
		return writeCIConfigOutput(out, *jsonOut)
	}

	out, err := buildCIConfigOutput()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "ci_config_failed", fmt.Sprintf("failed to load config: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		}
		return 1
	}
	return writeCIConfigOutput(out, *jsonOut)
}

func splitCommandSpec(raw string) (string, string) {
	if strings.Contains(raw, "=") {
		parts := strings.SplitN(raw, "=", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(raw)
}

func buildCIConfigResolvedOutput() (ciConfigOutput, error) {
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return ciConfigOutput{}, err
	}
	source := "inferred"
	cmds := []string{}
	if cfg, ok, err := cicmd.LoadConfig(); err == nil && ok && len(cfg.Commands) > 0 {
		cmds = append(cmds, formatCICommandSpecs(cfg.Commands)...)
		source = ".jul/ci.toml"
	} else if err == nil {
		cmds = cicmd.InferDefaultCommands(root)
	} else {
		return ciConfigOutput{}, err
	}
	return ciConfigOutput{
		Status:   "ok",
		Source:   source,
		Commands: cmds,
		Resolved: true,
	}, nil
}

type ciConfigOutput struct {
	Status          string   `json:"status"`
	Message         string   `json:"message,omitempty"`
	RunOnCheckpoint *bool    `json:"run_on_checkpoint,omitempty"`
	RunOnDraft      *bool    `json:"run_on_draft,omitempty"`
	DraftBlocking   *bool    `json:"draft_ci_blocking,omitempty"`
	Source          string   `json:"source,omitempty"`
	Commands        []string `json:"commands,omitempty"`
	Resolved        bool     `json:"resolved,omitempty"`
}

func buildCIConfigOutput() (ciConfigOutput, error) {
	source, commands, err := resolveCIConfigCommands()
	if err != nil {
		return ciConfigOutput{}, err
	}
	runOnCheckpoint := config.CIRunOnCheckpoint()
	runOnDraft := config.CIRunOnDraft()
	draftBlocking := config.CIDraftBlocking()
	return ciConfigOutput{
		Status:          "ok",
		RunOnCheckpoint: &runOnCheckpoint,
		RunOnDraft:      &runOnDraft,
		DraftBlocking:   &draftBlocking,
		Source:          source,
		Commands:        commands,
	}, nil
}

func resolveCIConfigCommands() (string, []string, error) {
	if cfg, ok, err := cicmd.LoadConfig(); err == nil && ok && len(cfg.Commands) > 0 {
		return ".jul/ci.toml", formatCICommandSpecs(cfg.Commands), nil
	} else if err != nil {
		return "", nil, err
	}
	return "default", []string{"go test ./..."}, nil
}

func formatCICommandSpecs(cmds []cicmd.CommandSpec) []string {
	labels := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		if strings.TrimSpace(cmd.Command) == "" {
			continue
		}
		label := cmd.Command
		if strings.TrimSpace(cmd.Name) != "" {
			label = fmt.Sprintf("%s: %s", cmd.Name, cmd.Command)
		}
		labels = append(labels, label)
	}
	return labels
}

func writeCIConfigOutput(out ciConfigOutput, jsonOut bool) int {
	if jsonOut {
		return writeJSON(out)
	}
	renderCIConfigOutput(out)
	return 0
}

func renderCIConfigOutput(out ciConfigOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
		return
	}
	if out.Resolved {
		fmt.Fprintln(os.Stdout, "CI configuration (resolved):")
		if out.Source != "" {
			fmt.Fprintf(os.Stdout, "  source: %s\n", out.Source)
		}
		if len(out.Commands) > 0 {
			fmt.Fprintln(os.Stdout, "  commands:")
			for _, cmd := range out.Commands {
				fmt.Fprintf(os.Stdout, "    - %s\n", cmd)
			}
		}
		return
	}
	fmt.Fprintln(os.Stdout, "CI configuration:")
	if out.RunOnCheckpoint != nil {
		fmt.Fprintf(os.Stdout, "  run_on_checkpoint: %t\n", *out.RunOnCheckpoint)
	}
	if out.RunOnDraft != nil {
		fmt.Fprintf(os.Stdout, "  run_on_draft: %t\n", *out.RunOnDraft)
	}
	if out.DraftBlocking != nil {
		fmt.Fprintf(os.Stdout, "  draft_ci_blocking: %t\n", *out.DraftBlocking)
	}
	if out.Source == "default" && len(out.Commands) > 0 {
		fmt.Fprintf(os.Stdout, "  default command: %s\n", out.Commands[0])
		return
	}
	if len(out.Commands) > 0 {
		label := "commands"
		if out.Source != "" {
			label = fmt.Sprintf("commands (%s)", out.Source)
		}
		fmt.Fprintf(os.Stdout, "  %s:\n", label)
		for _, cmd := range out.Commands {
			fmt.Fprintf(os.Stdout, "    - %s\n", cmd)
		}
	}
}

type ciCancelOutput struct {
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	CommitSHA string `json:"commit_sha,omitempty"`
	Cancelled bool   `json:"cancelled"`
}

func writeCICancelOutput(out ciCancelOutput, jsonOut bool) int {
	if jsonOut {
		return writeJSON(out)
	}
	renderCICancelOutput(out)
	return 0
}

func renderCICancelOutput(out ciCancelOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
	}
}

func runCICancel(args []string) int {
	fs, jsonOut := newFlagSet("ci cancel")
	_ = fs.Parse(args)

	running, err := cicmd.ReadRunning()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "ci_cancel_failed", fmt.Sprintf("failed to read running CI: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to read running CI: %v\n", err)
		}
		return 1
	}
	if running == nil || running.PID == 0 {
		out := ciCancelOutput{
			Status:    "ok",
			Cancelled: false,
			Message:   "No running CI job.",
		}
		return writeCICancelOutput(out, *jsonOut)
	}
	_ = cancelRunningCI(running)
	out := ciCancelOutput{
		Status:    "ok",
		Cancelled: true,
		CommitSHA: running.CommitSHA,
		Message:   fmt.Sprintf("Cancelled CI run for %s", running.CommitSHA),
	}
	return writeCICancelOutput(out, *jsonOut)
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
	fs, jsonOut := newFlagSet("ci list")
	limit := fs.Int("limit", 10, "Max runs to show")
	_ = fs.Parse(args)

	runs, err := cicmd.ListRuns(*limit)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "ci_list_failed", fmt.Sprintf("failed to list ci runs: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to list ci runs: %v\n", err)
		}
		return 1
	}
	payload := output.CIRunsJSON{Runs: runs}
	if *jsonOut {
		return writeJSON(payload)
	}
	output.RenderCIRuns(os.Stdout, payload.Runs, output.DefaultOptions())
	return 0
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
