package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	cicmd "github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func newCICommand() Command {
	return Command{
		Name:    "ci",
		Summary: "Run local CI and record attestation",
		Run: func(args []string) int {
			if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
				printCIUsage()
				return 0
			}

			sub := args[0]
			switch sub {
			case "run":
				return runCIRun(args[1:])
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

	att := client.Attestation{
		CommitSHA:   info.SHA,
		ChangeID:    changeID,
		Type:        *attType,
		Status:      result.Status,
		StartedAt:   result.StartedAt,
		FinishedAt:  result.FinishedAt,
		SignalsJSON: string(signals),
	}

	cli := client.New(config.BaseURL())
	created, err := cli.CreateAttestation(att)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to record attestation: %v\n", err)
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(created); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		if result.Status != "pass" {
			return 1
		}
		return 0
	}

	fmt.Fprintf(os.Stdout, "ci %s (commit %s)\n", result.Status, info.SHA)
	if result.Status != "pass" {
		return 1
	}
	return 0
}

func printCIUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul ci run [--cmd <command>] [--type ci] [--json]")
}
