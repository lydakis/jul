package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

func Commands(version string) []Command {
	return []Command{
		newStatusCommand(),
		newPromoteCommand(),
		newChangesCommand(),
		newVersionCommand(version),
	}
}

func newStatusCommand() Command {
	return Command{
		Name:    "status",
		Summary: "Show sync and attestation status (stub)",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("status", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			if *jsonOut {
				payload := map[string]any{
					"status":        "not_implemented",
					"workspace":     "",
					"sync_status":   "unknown",
					"checked_at_utc": time.Now().UTC().Format(time.RFC3339),
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(payload); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			fmt.Fprintln(os.Stdout, "jul status (stub)")
			fmt.Fprintln(os.Stdout, "  sync: unknown")
			fmt.Fprintln(os.Stdout, "  attestation: unknown")
			return 0
		},
	}
}

func newPromoteCommand() Command {
	return Command{
		Name:    "promote",
		Summary: "Promote a workspace to a branch (stub)",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("promote", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			toBranch := fs.String("to", "", "Target branch")
			force := fs.Bool("force", false, "Force promotion despite policy")
			_ = fs.Parse(args)

			if *toBranch == "" {
				fmt.Fprintln(os.Stderr, "missing --to <branch>")
				return 1
			}

			fmt.Fprintf(os.Stdout, "promote (stub): to=%s force=%t\n", *toBranch, *force)
			return 0
		},
	}
}

func newChangesCommand() Command {
	return Command{
		Name:    "changes",
		Summary: "List changes (stub)",
		Run: func(args []string) int {
			_ = args
			fmt.Fprintln(os.Stdout, "jul changes (stub)")
			fmt.Fprintln(os.Stdout, "  no data (server integration not wired yet)")
			return 0
		},
	}
}

func newVersionCommand(version string) Command {
	return Command{
		Name:    "version",
		Summary: "Show CLI version",
		Run: func(args []string) int {
			_ = args
			fmt.Fprintln(os.Stdout, version)
			return 0
		},
	}
}
