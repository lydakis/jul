package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/hooks"
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
		newReviewCommand(),
		newSyncCommand(),
		newStatusCommand(),
		newLogCommand(),
		newDiffCommand(),
		newShowCommand(),
		newApplyCommand(),
		newReflogCommand(),
		newPromoteCommand(),
		newChangesCommand(),
		newQueryCommand(),
		newSuggestionsCommand(),
		newSuggestCommand(),
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

			status, err := buildLocalStatus()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read status: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(status); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			output.RenderStatus(os.Stdout, status, output.DefaultOptions())
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
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			entries, err := localReflogEntries(*limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to fetch reflog: %v\n", err)
				return 1
			}
			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(entries); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			if len(entries) == 0 {
				fmt.Fprintln(os.Stdout, "No reflog entries.")
				return 0
			}
			for _, entry := range entries {
				if entry.Kind == "draft" {
					fmt.Fprintf(os.Stdout, "        └─ draft sync (%s)\n", entry.When)
					continue
				}
				msg := entry.Message
				if msg == "" {
					msg = "checkpoint"
				}
				fmt.Fprintf(os.Stdout, "%s checkpoint \"%s\" (%s)\n", entry.CommitSHA, msg, entry.When)
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

			shaArg := strings.TrimSpace(fs.Arg(0))
			var targetSHA string
			if shaArg != "" {
				resolved, err := gitutil.Git("rev-parse", shaArg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to resolve commit: %v\n", err)
					return 1
				}
				targetSHA = strings.TrimSpace(resolved)
			} else if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
				targetSHA = checkpoint.SHA
			} else {
				current, err := gitutil.CurrentCommit()
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
					return 1
				}
				targetSHA = current.SHA
			}
			if targetSHA == "" {
				fmt.Fprintln(os.Stderr, "failed to resolve commit to promote")
				return 1
			}
			if !config.BaseURLConfigured() {
				if err := promoteLocal(*toBranch, targetSHA, *force); err != nil {
					fmt.Fprintf(os.Stderr, "promote failed: %v\n", err)
					return 1
				}
			} else {
				cli := client.New(config.BaseURL())
				if err := cli.Promote(config.WorkspaceID(), *toBranch, targetSHA, *force); err != nil {
					fmt.Fprintf(os.Stderr, "promote failed: %v\n", err)
					return 1
				}
			}

			if *force {
				if config.BaseURLConfigured() {
					fmt.Fprintln(os.Stdout, "promote requested (force)")
					return 0
				}
				fmt.Fprintln(os.Stdout, "promote completed (force)")
				return 0
			}

			if config.BaseURLConfigured() {
				fmt.Fprintln(os.Stdout, "promote requested")
				return 0
			}
			fmt.Fprintln(os.Stdout, "promote completed")
			return 0
		},
	}
}

func promoteLocal(branch, sha string, force bool) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch required")
	}
	if strings.TrimSpace(sha) == "" {
		return fmt.Errorf("commit sha required")
	}
	ref := "refs/heads/" + strings.TrimSpace(branch)
	if !force && gitutil.RefExists(ref) {
		current, err := gitutil.ResolveRef(ref)
		if err != nil {
			return err
		}
		current = strings.TrimSpace(current)
		if current != "" {
			if _, err := gitutil.Git("merge-base", "--is-ancestor", current, sha); err != nil {
				return fmt.Errorf("promote would not be fast-forward; use --force to override")
			}
		}
	}
	return gitutil.UpdateRef(ref, sha)
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

			var changes []client.Change
			var err error
			if !config.BaseURLConfigured() {
				changes, err = localChanges()
			} else {
				cli := client.New(config.BaseURL())
				changes, err = cli.ListChanges()
			}
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
			output.RenderChanges(os.Stdout, changes, output.DefaultOptions())
			return 0
		},
	}
}

func localChanges() ([]client.Change, error) {
	entries, err := listCheckpoints()
	if err != nil {
		return nil, err
	}
	type changeSummary struct {
		change    client.Change
		count     int
		latestAt  time.Time
		earliest  time.Time
		hasLatest bool
	}
	byChange := make(map[string]*changeSummary)
	for _, cp := range entries {
		changeID := cp.ChangeID
		if changeID == "" {
			changeID = gitutil.FallbackChangeID(cp.SHA)
		}
		summary, ok := byChange[changeID]
		if !ok {
			summary = &changeSummary{
				change: client.Change{
					ChangeID: changeID,
					Author:   cp.Author,
					Status:   "open",
				},
			}
			byChange[changeID] = summary
		}
		summary.count++
		if !summary.hasLatest || cp.When.After(summary.latestAt) {
			summary.latestAt = cp.When
			summary.hasLatest = true
			summary.change.LatestRevision.CommitSHA = cp.SHA
			if title := firstLine(cp.Message); title != "" {
				summary.change.Title = title
			}
		}
		if summary.earliest.IsZero() || cp.When.Before(summary.earliest) {
			summary.earliest = cp.When
		}
	}
	summaries := make([]*changeSummary, 0, len(byChange))
	for _, summary := range byChange {
		summary.change.RevisionCount = summary.count
		summary.change.LatestRevision.RevIndex = summary.count
		summary.change.CreatedAt = summary.earliest
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].latestAt.After(summaries[j].latestAt)
	})
	changes := make([]client.Change, 0, len(summaries))
	for _, summary := range summaries {
		changes = append(changes, summary.change)
	}
	return changes, nil
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
