package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/hooks"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func Commands(version string) []Command {
	return []Command{
		newCloneCommand(),
		newInitCommand(),
		newRemoteCommand(),
		newConfigureCommand(),
		newWorkspaceCommand(),
		newCheckpointCommand(),
		newSyncCommand(),
		newStatusCommand(),
		newReflogCommand(),
		newPromoteCommand(),
		newChangesCommand(),
		newQueryCommand(),
		newSuggestionsCommand(),
		newSuggestCommand(),
		newSuggestionActionCommand("accept", "accept"),
		newSuggestionActionCommand("reject", "reject"),
		newCICommand(),
		newHooksCommand(),
		newVersionCommand(version),
	}
}

func newSyncCommand() Command {
	return Command{
		Name:    "sync",
		Summary: "Sync draft locally and optionally to remote",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("sync", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			res, err := syncer.Sync()
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			fmt.Fprintln(os.Stdout, "Syncing...")
			fmt.Fprintf(os.Stdout, "  ✓ Draft committed (%s)\n", res.DraftSHA)
			if res.RemoteName == "" {
				fmt.Fprintln(os.Stdout, "  ✓ Workspace ref updated (local)")
				if res.RemoteProblem != "" {
					fmt.Fprintf(os.Stdout, "  (%s)\n", res.RemoteProblem)
				} else {
					fmt.Fprintln(os.Stdout, "  (No remote configured)")
				}
				return 0
			}
			fmt.Fprintf(os.Stdout, "  ✓ Sync ref pushed (%s)\n", res.SyncRef)
			if res.Diverged {
				fmt.Fprintln(os.Stdout, "  ⚠ Workspace diverged — run 'jul merge' when ready")
				return 0
			}
			if res.AutoMerged {
				fmt.Fprintln(os.Stdout, "  ✓ Auto-merged (no conflicts)")
			}
			if res.WorkspaceUpdated {
				fmt.Fprintln(os.Stdout, "  ✓ Workspace ref updated")
			}
			return 0
		},
	}
}

func newStatusCommand() Command {
	return Command{
		Name:    "status",
		Summary: "Show sync and attestation status",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("status", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			info, err := gitutil.CurrentCommit()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
				return 1
			}
			repoName := config.RepoName()
			if repoName != "" {
				info.RepoName = repoName
			}

			wsID := config.WorkspaceID()
			att, err := metadata.GetAttestation(info.SHA)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to fetch attestation: %v\n", err)
				return 1
			}

			payload := output.BuildStatusPayload(wsID, info.RepoName, info.Branch, info.SHA, info.ChangeID, client.Workspace{}, att)
			payload.SyncStatus = "local"
			if err := output.RenderStatus(os.Stdout, payload, *jsonOut); err != nil {
				fmt.Fprintf(os.Stderr, "failed to render status: %v\n", err)
				return 1
			}
			return 0
		},
	}
}

func newReflogCommand() Command {
	return Command{
		Name:    "reflog",
		Summary: "Show recent workspace history",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("reflog", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			limit := fs.Int("limit", 20, "Max entries to show")
			_ = fs.Parse(args)

			wsID := config.WorkspaceID()
			cli := client.New(config.BaseURL())
			entries, err := cli.Reflog(wsID, *limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to fetch reflog: %v\n", err)
				return 1
			}

			if len(entries) == 0 {
				fmt.Fprintln(os.Stdout, "No reflog entries.")
				return 0
			}

			shown := 0
			for _, entry := range entries {
				fmt.Fprintf(os.Stdout, "%s %s (%s)\n", entry.CommitSHA, entry.ChangeID, entry.Source)
				shown++
				if *limit > 0 && shown >= *limit {
					break
				}
			}
			return 0
		},
	}
}

func newPromoteCommand() Command {
	return Command{
		Name:    "promote",
		Summary: "Promote a workspace to a branch",
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

			info, err := gitutil.CurrentCommit()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
				return 1
			}

			cli := client.New(config.BaseURL())
			if err := cli.Promote(config.WorkspaceID(), *toBranch, info.SHA, *force); err != nil {
				fmt.Fprintf(os.Stderr, "promote failed: %v\n", err)
				return 1
			}

			if *force {
				fmt.Fprintln(os.Stdout, "promote requested (force flag noted, server policy pending)")
				return 0
			}

			fmt.Fprintln(os.Stdout, "promote requested")
			return 0
		},
	}
}

func newChangesCommand() Command {
	return Command{
		Name:    "changes",
		Summary: "List changes",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("changes", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			cli := client.New(config.BaseURL())
			changes, err := cli.ListChanges()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to fetch changes: %v\n", err)
				return 1
			}
			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(changes); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}
			if len(changes) == 0 {
				fmt.Fprintln(os.Stdout, "No changes yet.")
				return 0
			}
			for _, ch := range changes {
				fmt.Fprintf(os.Stdout, "%s %s (rev %d, %s)\n", ch.ChangeID, ch.Title, ch.LatestRevision.RevIndex, ch.Status)
			}
			return 0
		},
	}
}

func newHooksCommand() Command {
	return Command{
		Name:    "hooks",
		Summary: "Manage git hooks",
		Run: func(args []string) int {
			if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
				printHooksUsage()
				return 0
			}

			sub := strings.ToLower(args[0])
			repoRoot, err := gitutil.RepoTopLevel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to locate git repo: %v\n", err)
				return 1
			}

			switch sub {
			case "install":
				cliCmd := os.Getenv("JUL_HOOK_CMD")
				if cliCmd == "" {
					cliCmd = "jul"
				}
				path, err := hooks.InstallPostCommit(repoRoot, cliCmd)
				if err != nil {
					fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
					return 1
				}
				fmt.Fprintf(os.Stdout, "installed post-commit hook: %s\n", path)
				return 0
			case "uninstall":
				if err := hooks.UninstallPostCommit(repoRoot); err != nil {
					fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
					return 1
				}
				fmt.Fprintln(os.Stdout, "removed post-commit hook")
				return 0
			case "status":
				installed, path, err := hooks.StatusPostCommit(repoRoot)
				if err != nil {
					fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
					return 1
				}
				if installed {
					fmt.Fprintf(os.Stdout, "post-commit hook installed: %s\n", path)
					return 0
				}
				fmt.Fprintf(os.Stdout, "post-commit hook not installed (%s)\n", path)
				return 1
			default:
				printHooksUsage()
				return 1
			}
		},
	}
}

func printHooksUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul hooks <install|uninstall|status>")
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
