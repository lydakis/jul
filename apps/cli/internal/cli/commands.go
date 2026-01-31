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
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
	"github.com/lydakis/jul/cli/internal/syncer"
	wsconfig "github.com/lydakis/jul/cli/internal/workspace"
)

func Commands(version string) []Command {
	return []Command{
		newCloneCommand(),
		newInitCommand(),
		newRemoteCommand(),
		newConfigureCommand(),
		newDoctorCommand(),
		newWorkspaceCommand(),
		newLocalCommand(),
		newCheckpointCommand(),
		newReviewCommand(),
		newSubmitCommand(),
		newTraceCommand(),
		newMergeCommand(),
		newSyncCommand(),
		newStatusCommand(),
		newLogCommand(),
		newDiffCommand(),
		newShowCommand(),
		newBlameCommand(),
		newApplyCommand(),
		newReflogCommand(),
		newPromoteCommand(),
		newChangesCommand(),
		newQueryCommand(),
		newSuggestionsCommand(),
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
				ciExit := maybeRunDraftCI(res, true)
				if ciExit != 0 {
					return ciExit
				}
				return 0
			}

			output.RenderSync(os.Stdout, res, output.DefaultOptions())
			ciExit := maybeRunDraftCI(res, false)
			if ciExit != 0 {
				return ciExit
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
			porcelain := fs.Bool("porcelain", false, "Output git status porcelain")
			_ = fs.Parse(args)

			if *porcelain {
				if *jsonOut {
					fmt.Fprintln(os.Stderr, "cannot combine --json with --porcelain")
					return 1
				}
				out, err := gitutil.Git("status", "--porcelain")
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to read git status: %v\n", err)
					return 1
				}
				if strings.TrimSpace(out) != "" {
					fmt.Fprintln(os.Stdout, out)
				}
				return 0
			}

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
			output.RenderReflog(os.Stdout, entries)
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
			noPolicy := fs.Bool("no-policy", false, "Skip promote policy checks")
			forceTarget := fs.Bool("force-target", false, "Dangerous: allow non-fast-forward update of target branch")
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
			if err := promoteWithStack(*toBranch, targetSHA, *forceTarget, *noPolicy); err != nil {
				fmt.Fprintf(os.Stderr, "promote failed: %v\n", err)
				return 1
			}

			if *forceTarget {
				fmt.Fprintln(os.Stdout, "promote completed (force-target)")
				return 0
			}

			if *noPolicy {
				fmt.Fprintln(os.Stdout, "promote completed (no-policy)")
				return 0
			}
			fmt.Fprintln(os.Stdout, "promote completed")
			return 0
		},
	}
}

func promoteLocal(branch, sha string, forceTarget bool, noPolicy bool) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch required")
	}
	if strings.TrimSpace(sha) == "" {
		return fmt.Errorf("commit sha required")
	}
	ref := "refs/heads/" + strings.TrimSpace(branch)
	_ = noPolicy
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	remoteTip, remoteName, err := fetchPublishTip(branch)
	if err != nil {
		return err
	}
	localTip := ""
	if gitutil.RefExists(ref) {
		if current, err := gitutil.ResolveRef(ref); err == nil {
			localTip = strings.TrimSpace(current)
		}
	}
	guardTip := strings.TrimSpace(remoteTip)
	if guardTip == "" {
		guardTip = localTip
	}
	if guardTip != "" && !forceTarget {
		if _, err := gitutil.Git("merge-base", "--is-ancestor", guardTip, sha); err != nil {
			return fmt.Errorf("promote would not be fast-forward; use --force-target to override")
		}
	}
	if remoteName != "" {
		if err := pushPublish(remoteName, sha, branch, forceTarget); err != nil {
			return err
		}
	}
	if err := gitutil.UpdateRef(ref, sha); err != nil {
		return err
	}
	if err := recordPromoteMeta(branch, sha); err != nil {
		return err
	}
	return startNewDraftAfterPromote(repoRoot, sha)
}

type stackWorkspace struct {
	User string
	Name string
}

func promoteWithStack(branch, targetSHA string, forceTarget, noPolicy bool) error {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	user, workspace := workspaceParts()
	stack, baseBranch, err := resolvePromoteStack(repoRoot, user, workspace)
	if err != nil {
		return err
	}
	if len(stack) == 1 {
		sha := strings.TrimSpace(targetSHA)
		if sha == "" {
			if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
				sha = checkpoint.SHA
			} else if current, err := gitutil.CurrentCommit(); err == nil {
				sha = current.SHA
			}
		}
		if sha == "" {
			return fmt.Errorf("failed to resolve commit to promote")
		}
		return promoteLocal(branch, sha, forceTarget, noPolicy)
	}
	if baseBranch != "" && strings.TrimSpace(branch) != strings.TrimSpace(baseBranch) {
		return fmt.Errorf("stacked workspace targets %s; use --to %s", baseBranch, baseBranch)
	}

	originalEnv := os.Getenv(config.EnvWorkspace)
	defer restoreWorkspaceEnv(originalEnv)

	// Promote bottom-up (base workspace first).
	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		wsID := entry.User + "/" + entry.Name
		if err := withWorkspaceEnv(wsID); err != nil {
			return err
		}
		sha := strings.TrimSpace(targetSHA)
		if i != 0 || sha == "" {
			sha = ""
			if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
				sha = checkpoint.SHA
			}
		}
		if strings.TrimSpace(sha) == "" {
			return fmt.Errorf("checkpoint required before promote for workspace %s", wsID)
		}
		if err := promoteLocal(branch, sha, forceTarget, noPolicy); err != nil {
			return err
		}
	}
	return nil
}

func resolvePromoteStack(repoRoot, user, workspace string) ([]stackWorkspace, string, error) {
	stack := []stackWorkspace{}
	seen := map[string]bool{}
	current := stackWorkspace{User: user, Name: workspace}
	for {
		key := current.User + "/" + current.Name
		if seen[key] {
			return nil, "", fmt.Errorf("workspace stack loop detected at %s", key)
		}
		seen[key] = true
		stack = append(stack, current)

		cfg, ok, err := wsconfig.ReadConfig(repoRoot, current.Name)
		if err != nil {
			return nil, "", err
		}
		if !ok || strings.TrimSpace(cfg.BaseRef) == "" {
			return stack, "", nil
		}
		baseRef, err := normalizeBaseRef(repoRoot, cfg.BaseRef)
		if err != nil {
			return nil, "", err
		}
	if strings.HasPrefix(baseRef, "refs/jul/workspaces/") {
		parentUser, parentWorkspace, ok := parseWorkspaceRef(baseRef)
		if !ok {
			return nil, "", fmt.Errorf("invalid workspace ref %s", baseRef)
		}
		current = stackWorkspace{User: parentUser, Name: parentWorkspace}
		continue
	}
	if strings.HasPrefix(baseRef, "refs/jul/changes/") {
		changeID := strings.TrimPrefix(baseRef, "refs/jul/changes/")
		parentUser, parentWorkspace, ok := findWorkspaceForChange(changeID)
		if !ok {
			return nil, "", fmt.Errorf("could not resolve parent workspace for change %s", changeID)
		}
		current = stackWorkspace{User: parentUser, Name: parentWorkspace}
		continue
	}
		if strings.HasPrefix(baseRef, "refs/heads/") {
			return stack, strings.TrimPrefix(baseRef, "refs/heads/"), nil
		}
		return stack, baseRef, nil
	}
}

func parseWorkspaceRef(ref string) (string, string, bool) {
	const prefix = "refs/jul/workspaces/"
	if !strings.HasPrefix(ref, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(ref, prefix), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func findWorkspaceForChange(changeID string) (string, string, bool) {
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		return "", "", false
	}
	refsOut, err := gitutil.Git("show-ref")
	if err != nil {
		return "", "", false
	}
	lines := strings.Split(strings.TrimSpace(refsOut), "\n")
	prefix := "refs/jul/keep/"
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ref := fields[1]
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		rest := strings.TrimPrefix(ref, prefix)
		parts := strings.Split(rest, "/")
		if len(parts) < 3 {
			continue
		}
		user := parts[0]
		workspace := parts[1]
		refChange := parts[2]
		if refChange != changeID {
			continue
		}
		return user, workspace, true
	}
	return "", "", false
}

func withWorkspaceEnv(wsID string) error {
	return os.Setenv(config.EnvWorkspace, wsID)
}

func restoreWorkspaceEnv(prev string) {
	if strings.TrimSpace(prev) == "" {
		_ = os.Unsetenv(config.EnvWorkspace)
		return
	}
	_ = os.Setenv(config.EnvWorkspace, prev)
}

func recordPromoteMeta(branch, sha string) error {
	if strings.TrimSpace(sha) == "" {
		return nil
	}
	changeID := changeIDForCommit(sha)
	anchorSHA, checkpoints, err := changeMetaFromCheckpoints(changeID)
	if err != nil {
		return err
	}
	if anchorSHA == "" {
		anchorSHA = sha
	}

	meta, ok, err := metadata.ReadChangeMeta(anchorSHA)
	if err != nil {
		return err
	}
	if !ok {
		meta = metadata.ChangeMeta{}
	}
	if meta.ChangeID == "" {
		meta.ChangeID = changeID
	}
	if meta.AnchorSHA == "" {
		meta.AnchorSHA = anchorSHA
	}
	if len(checkpoints) > 0 {
		meta.Checkpoints = checkpoints
	}
	meta.PromoteEvents = append(meta.PromoteEvents, metadata.PromoteEvent{
		Target:    strings.TrimSpace(branch),
		Strategy:  "fast-forward",
		Timestamp: time.Now().UTC(),
		Published: []string{sha},
	})
	return metadata.WriteChangeMeta(meta)
}

func startNewDraftAfterPromote(repoRoot, publishedSHA string) error {
	if strings.TrimSpace(publishedSHA) == "" {
		return nil
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return err
	}
	newChangeID, err := gitutil.NewChangeID()
	if err != nil {
		return err
	}
	treeSHA, err := gitutil.TreeOf(publishedSHA)
	if err != nil {
		return err
	}
	newDraftSHA, err := gitutil.CreateDraftCommitFromTree(treeSHA, publishedSHA, newChangeID)
	if err != nil {
		return err
	}
	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)
	if err := gitutil.UpdateRef(syncRef, newDraftSHA); err != nil {
		return err
	}
	if err := gitutil.UpdateRef(workspaceRef, publishedSHA); err != nil {
		return err
	}
	if err := writeWorkspaceLease(repoRoot, workspace, publishedSHA); err != nil {
		return err
	}
	if err := gitutil.EnsureHeadRef(repoRoot, workspaceHeadRef(workspace), publishedSHA); err != nil {
		return err
	}
	if _, err := gitutil.Git("read-tree", "--reset", "-u", newDraftSHA); err != nil {
		return err
	}
	if _, err := gitutil.Git("clean", "-fd", "--exclude=.jul"); err != nil {
		return err
	}
	return nil
}

func fetchPublishTip(branch string) (string, string, error) {
	remote, err := remotesel.Resolve()
	if err != nil {
		switch err {
		case remotesel.ErrNoRemote, remotesel.ErrRemoteMissing:
			return "", "", nil
		default:
			return "", "", err
		}
	}
	if strings.TrimSpace(remote.Name) == "" {
		return "", "", nil
	}
	ref := "refs/heads/" + strings.TrimSpace(branch)
	out, err := gitutil.Git("ls-remote", remote.Name, ref)
	if err != nil {
		return "", "", err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return "", remote.Name, nil
	}
	return strings.TrimSpace(fields[0]), remote.Name, nil
}

func pushPublish(remoteName, sha, branch string, forceTarget bool) error {
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	ref := "refs/heads/" + strings.TrimSpace(branch)
	spec := fmt.Sprintf("%s:%s", strings.TrimSpace(sha), ref)
	args := []string{"push"}
	if forceTarget {
		args = append(args, "--force")
	}
	args = append(args, remoteName, spec)
	_, err := gitutil.Git(args...)
	return err
}

func changeIDForCommit(sha string) string {
	message, _ := gitutil.CommitMessage(sha)
	changeID := gitutil.ExtractChangeID(message)
	if changeID != "" {
		return changeID
	}
	if checkpoint, _ := checkpointForSHA(sha); checkpoint != nil && checkpoint.ChangeID != "" {
		return checkpoint.ChangeID
	}
	return gitutil.FallbackChangeID(sha)
}

func checkpointForSHA(sha string) (*checkpointInfo, error) {
	entries, err := listCheckpoints()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.SHA == sha {
			return &entry, nil
		}
	}
	return nil, nil
}

func changeMetaFromCheckpoints(changeID string) (string, []metadata.ChangeCheckpoint, error) {
	if strings.TrimSpace(changeID) == "" {
		return "", nil, nil
	}
	entries, err := listCheckpoints()
	if err != nil {
		return "", nil, err
	}
	type entry struct {
		metadata.ChangeCheckpoint
		when time.Time
	}
	matched := make([]entry, 0)
	for _, cp := range entries {
		if cp.ChangeID != changeID {
			continue
		}
		matched = append(matched, entry{
			ChangeCheckpoint: metadata.ChangeCheckpoint{
				SHA:     cp.SHA,
				Message: firstLine(cp.Message),
			},
			when: cp.When,
		})
	}
	if len(matched) == 0 {
		return "", nil, nil
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].when.Before(matched[j].when)
	})
	checkpoints := make([]metadata.ChangeCheckpoint, 0, len(matched))
	for _, cp := range matched {
		checkpoints = append(checkpoints, cp.ChangeCheckpoint)
	}
	return matched[0].SHA, checkpoints, nil
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

			changes, err := localChanges()
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
