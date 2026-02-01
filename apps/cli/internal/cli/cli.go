package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
	"github.com/lydakis/jul/cli/internal/syncer"
)

type Command struct {
	Name    string
	Summary string
	Run     func(args []string) int
}

type App struct {
	Commands []Command
	Version  string
}

func (a *App) Run(args []string) int {
	jsonOut, args := stripJSONFlag(args)
	if len(args) == 0 {
		return a.usageWithJSON("missing command", jsonOut)
	}

	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return a.usageWithJSON("", jsonOut)
	}

	for _, cmd := range a.Commands {
		if cmd.Name == args[0] {
			cmdArgs := args[1:]
			if jsonOut {
				cmdArgs = ensureJSONFlag(cmdArgs)
			}
			if shouldAutoSync(cmd.Name) && config.SyncMode() == "on-command" {
				if os.Getenv("JUL_NO_SYNC") == "" {
					if _, err := gitutil.RepoTopLevel(); err == nil {
						if _, err := syncer.Sync(); err != nil {
							fmt.Fprintf(os.Stderr, "sync warning: %v\n", err)
						}
					}
				}
			}
			return cmd.Run(cmdArgs)
		}
	}

	return a.usageWithJSON(fmt.Sprintf("unknown command: %s", args[0]), jsonOut)
}

func (a *App) usage(problem string) int {
	return a.usageWithJSON(problem, false)
}

type usageOutput struct {
	Version  string          `json:"version"`
	Usage    string          `json:"usage"`
	Commands []commandOutput `json:"commands,omitempty"`
}

type commandOutput struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

func (a *App) usageWithJSON(problem string, jsonOut bool) int {
	if jsonOut {
		if problem != "" {
			_ = output.EncodeError(os.Stdout, "usage_error", problem, []output.NextAction{
				{Action: "help", Command: "jul help --json"},
			})
			return 1
		}
		commands := make([]commandOutput, 0, len(a.Commands))
		for _, cmd := range a.Commands {
			commands = append(commands, commandOutput{Name: cmd.Name, Summary: cmd.Summary})
		}
		out := usageOutput{
			Version:  a.Version,
			Usage:    "jul <command> [options]",
			Commands: commands,
		}
		if err := output.EncodeJSON(os.Stdout, out); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		return 0
	}
	if problem != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n\n", problem)
	}

	fmt.Fprintf(os.Stderr, "jul %s\n", a.Version)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  jul <command> [options]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	for _, cmd := range a.Commands {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", cmd.Name, cmd.Summary)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jul help' to see this message.")

	if problem == "" {
		return 0
	}
	return 1
}

func FormatLines(lines ...string) string {
	return strings.Join(lines, "\n")
}

func shouldAutoSync(command string) bool {
	if command == "" {
		return false
	}
	return !autoSyncSkip[strings.ToLower(command)]
}

var autoSyncSkip = map[string]bool{
	"accept":      true,
	"clone":       true,
	"doctor":      true,
	"init":        true,
	"merge":       true,
	"reject":      true,
	"suggestions": true,
	"sync":        true,
	"version":     true,
}
