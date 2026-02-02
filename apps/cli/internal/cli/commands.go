package cli

import (
	"errors"
	"fmt"
	"io"
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
	"github.com/lydakis/jul/cli/internal/policy"
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
		newDraftCommand(),
		newReviewCommand(),
		newSubmitCommand(),
		newTraceCommand(),
		newMergeCommand(),
		newSyncCommand(),
		newStatusCommand(),
		newLogCommand(),
		newDiffCommand(),
		newShowCommand(),
		newPruneCommand(),
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
			fs, jsonOut := newFlagSet("sync")
			allowSecrets := fs.Bool("allow-secrets", false, "Allow draft push even if secrets are detected")
			daemon := fs.Bool("daemon", false, "Run sync continuously in the foreground")
			jsonRequested := hasJSONFlag(args)
			if jsonRequested {
				fs.SetOutput(io.Discard)
			}
			if err := fs.Parse(args); err != nil {
				if jsonRequested {
					_ = output.EncodeError(os.Stdout, "sync_invalid_args", err.Error(), nil)
				}
				return 1
			}

			if *daemon {
				return runSyncDaemon(syncer.SyncOptions{AllowSecrets: *allowSecrets})
			}

			res, err := syncer.SyncWithOptions(syncer.SyncOptions{AllowSecrets: *allowSecrets})
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "sync_failed", err.Error(), []output.NextAction{
						{Action: "retry", Command: "jul sync --json"},
					})
				} else {
					fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
				}
				return 1
			}

			if *jsonOut {
				if code := writeJSON(res); code != 0 {
					return code
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
			fs, jsonOut := newFlagSet("status")
			porcelain := fs.Bool("porcelain", false, "Output git status porcelain")
			_ = fs.Parse(args)

			if *porcelain {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "status_incompatible_flags", "cannot combine --json with --porcelain", nil)
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
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "status_failed", fmt.Sprintf("failed to read status: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to read status: %v\n", err)
				}
				return 1
			}

			if *jsonOut {
				return writeJSON(status)
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
			fs, jsonOut := newFlagSet("reflog")
			limit := fs.Int("limit", 20, "Max entries to show")
			_ = fs.Parse(args)

			entries, err := localReflogEntries(*limit)
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "reflog_failed", fmt.Sprintf("failed to fetch reflog: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to fetch reflog: %v\n", err)
				}
				return 1
			}
			if *jsonOut {
				return writeJSON(entries)
			}
			output.RenderReflog(os.Stdout, entries)
			return 0
		},
	}
}

type promoteOutput struct {
	Status      string   `json:"status"`
	Branch      string   `json:"branch"`
	CommitSHA   string   `json:"commit_sha"`
	Strategy    string   `json:"strategy,omitempty"`
	Published   []string `json:"published,omitempty"`
	BaseMarker  string   `json:"base_marker_sha,omitempty"`
	ForceTarget bool     `json:"force_target,omitempty"`
	NoPolicy    bool     `json:"no_policy,omitempty"`
}

type promoteOptions struct {
	Branch         string
	TargetSHA      string
	Strategy       string
	ForceTarget    bool
	NoPolicy       bool
	ConfirmRewrite bool
}

type promoteResult struct {
	Branch        string
	TargetSHA     string
	Strategy      string
	Published     []string
	PublishedTip  string
	BaseMarkerSHA string
	ForceTarget   bool
	NoPolicy      bool
}

type promoteError struct {
	Code    string
	Message string
	Next    []output.NextAction
}

func (e promoteError) Error() string {
	return e.Message
}

func newPromoteCommand() Command {
	return Command{
		Name:    "promote",
		Summary: "Promote a workspace to a branch",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("promote")
			toBranch := fs.String("to", "", "Target branch")
			noPolicy := fs.Bool("no-policy", false, "Skip promote policy checks")
			forceTarget := fs.Bool("force-target", false, "Dangerous: allow non-fast-forward update of target branch")
			rebase := fs.Bool("rebase", false, "Rebase checkpoints onto target")
			squash := fs.Bool("squash", false, "Squash checkpoints into single commit")
			merge := fs.Bool("merge", false, "Create merge commit on target")
			confirmRewrite := fs.Bool("confirm-rewrite", false, "Confirm publishing to rewritten target")
			jsonRequested := hasJSONFlag(args)
			if jsonRequested {
				fs.SetOutput(io.Discard)
			}
			if err := fs.Parse(args); err != nil {
				if jsonRequested {
					_ = output.EncodeError(os.Stdout, "promote_invalid_args", err.Error(), nil)
				}
				return 1
			}

			if *toBranch == "" {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "promote_missing_target", "missing --to <branch>", nil)
				} else {
					fmt.Fprintln(os.Stderr, "missing --to <branch>")
				}
				return 1
			}

			strategy := ""
			switch {
			case *rebase:
				strategy = "rebase"
			case *squash:
				strategy = "squash"
			case *merge:
				strategy = "merge"
			}
			if (*rebase && *squash) || (*rebase && *merge) || (*squash && *merge) {
				err := promoteError{
					Code:    "promote_invalid_strategy",
					Message: "choose only one of --rebase, --squash, or --merge",
				}
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, err.Code, err.Message, nil)
				} else {
					fmt.Fprintln(os.Stderr, err.Message)
				}
				return 1
			}

			shaArg := strings.TrimSpace(fs.Arg(0))
			var targetSHA string
			if shaArg != "" {
				resolved, err := gitutil.Git("rev-parse", shaArg)
				if err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "promote_resolve_failed", fmt.Sprintf("failed to resolve commit: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to resolve commit: %v\n", err)
					}
					return 1
				}
				targetSHA = strings.TrimSpace(resolved)
			} else if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
				targetSHA = checkpoint.SHA
			} else {
				current, err := gitutil.CurrentCommit()
				if err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "promote_git_state_failed", fmt.Sprintf("failed to read git state: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
					}
					return 1
				}
				targetSHA = current.SHA
			}
			if targetSHA == "" {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "promote_missing_commit", "failed to resolve commit to promote", nil)
				} else {
					fmt.Fprintln(os.Stderr, "failed to resolve commit to promote")
				}
				return 1
			}
			res, err := promoteWithStack(promoteOptions{
				Branch:         *toBranch,
				TargetSHA:      targetSHA,
				Strategy:       strategy,
				ForceTarget:    *forceTarget,
				NoPolicy:       *noPolicy,
				ConfirmRewrite: *confirmRewrite,
			})
			if err != nil {
				var perr promoteError
				if errors.As(err, &perr) {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, perr.Code, perr.Message, perr.Next)
					} else {
						fmt.Fprintln(os.Stderr, perr.Message)
					}
					return 1
				}
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "promote_failed", err.Error(), nil)
				} else {
					fmt.Fprintf(os.Stderr, "promote failed: %v\n", err)
				}
				return 1
			}

			out := promoteOutput{
				Status:      "ok",
				Branch:      res.Branch,
				CommitSHA:   res.TargetSHA,
				Strategy:    res.Strategy,
				Published:   res.Published,
				BaseMarker:  res.BaseMarkerSHA,
				ForceTarget: res.ForceTarget,
				NoPolicy:    res.NoPolicy,
			}
			if *jsonOut {
				return writeJSON(out)
			}
			renderPromoteOutput(out)
			return 0
		},
	}
}

func renderPromoteOutput(out promoteOutput) {
	suffix := ""
	switch {
	case out.ForceTarget:
		suffix = "force-target"
	case out.NoPolicy:
		suffix = "no-policy"
	case strings.TrimSpace(out.Strategy) != "" && strings.TrimSpace(out.Strategy) != "rebase":
		suffix = strings.TrimSpace(out.Strategy)
	}
	if suffix != "" {
		fmt.Fprintf(os.Stdout, "promote completed (%s)\n", suffix)
		return
	}
	fmt.Fprintln(os.Stdout, "promote completed")
}

func promoteLocal(opts promoteOptions) (promoteResult, error) {
	branch := strings.TrimSpace(opts.Branch)
	if branch == "" {
		return promoteResult{}, fmt.Errorf("branch required")
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return promoteResult{}, err
	}

	sha := strings.TrimSpace(opts.TargetSHA)
	if sha == "" {
		if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
			sha = checkpoint.SHA
		} else if current, err := gitutil.CurrentCommit(); err == nil {
			sha = current.SHA
		}
	}
	if strings.TrimSpace(sha) == "" {
		return promoteResult{}, fmt.Errorf("commit sha required")
	}

	changeID := changeIDForCommit(sha)
	anchorSHA, checkpoints, err := changeMetaFromCheckpoints(changeID)
	if err != nil {
		return promoteResult{}, err
	}
	if len(checkpoints) > 0 {
		filtered := make([]metadata.ChangeCheckpoint, 0, len(checkpoints))
		for _, cp := range checkpoints {
			filtered = append(filtered, cp)
			if strings.TrimSpace(cp.SHA) == strings.TrimSpace(sha) {
				break
			}
		}
		if len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1].SHA) == strings.TrimSpace(sha) {
			checkpoints = filtered
		}
	}
	if len(checkpoints) == 0 {
		msg, _ := gitutil.CommitMessage(sha)
		checkpoints = []metadata.ChangeCheckpoint{{SHA: sha, Message: firstLine(msg)}}
		anchorSHA = sha
	}
	if anchorSHA == "" {
		anchorSHA = checkpoints[0].SHA
	}
	checkpointSHAs := make([]string, 0, len(checkpoints))
	for _, cp := range checkpoints {
		checkpointSHAs = append(checkpointSHAs, strings.TrimSpace(cp.SHA))
	}

	policyCfg, policyOK, err := policy.LoadPromotePolicy(repoRoot, branch)
	if err != nil {
		return promoteResult{}, err
	}
	strategy, err := resolvePromoteStrategy(opts.Strategy, policyCfg.Strategy, policyOK)
	if err != nil {
		return promoteResult{}, err
	}
	if !opts.NoPolicy && policyOK {
		if err := enforcePromotePolicy(policyCfg, sha, changeID); err != nil {
			return promoteResult{}, err
		}
	}

	remoteTip, remoteName, err := fetchPublishTip(branch)
	if err != nil {
		return promoteResult{}, err
	}
	ref := "refs/heads/" + strings.TrimSpace(branch)
	localTip := ""
	if gitutil.RefExists(ref) {
		if current, err := gitutil.ResolveRef(ref); err == nil {
			localTip = strings.TrimSpace(current)
		}
	}

	_, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	trackTip := ""
	if cfg, ok, err := wsconfig.ReadConfig(repoRoot, workspace); err == nil && ok {
		trackTip = strings.TrimSpace(cfg.TrackTip)
	}
	if trackTip != "" && remoteTip != "" && !gitutil.IsAncestor(trackTip, remoteTip) {
		if !opts.ConfirmRewrite {
			return promoteResult{}, promoteError{
				Code:    "promote_target_rewritten",
				Message: fmt.Sprintf("promote blocked: target %s was rewritten; use --confirm-rewrite after restacking", branch),
				Next: []output.NextAction{
					{Action: "restack", Command: fmt.Sprintf("jul ws restack --onto %s", branch)},
					{Action: "confirm", Command: fmt.Sprintf("jul promote --to %s --confirm-rewrite", branch)},
				},
			}
		}
	}

	published := []string{}
	publishedTip := ""
	var mergeCommitSHA *string
	var mainline *int

	switch strategy {
	case "rebase":
		if remoteTip == "" || gitutil.IsAncestor(remoteTip, sha) || opts.ForceTarget {
			published = append(published, checkpointSHAs...)
			publishedTip = published[len(published)-1]
		} else {
			published, err = promoteRebase(repoRoot, remoteTip, checkpointSHAs)
			if err != nil {
				return promoteResult{}, err
			}
			if len(published) == 0 {
				return promoteResult{}, fmt.Errorf("rebase produced no published commits")
			}
			publishedTip = published[len(published)-1]
		}
	case "squash":
		baseTip := resolvePromoteBaseTip(remoteTip, localTip, checkpoints)
		msg, _ := gitutil.CommitMessage(checkpoints[len(checkpoints)-1].SHA)
		msg = ensureChangeID(msg, changeID)
		publishedTip, err = promoteSquash(repoRoot, baseTip, checkpointSHAs, msg)
		if err != nil {
			return promoteResult{}, err
		}
		published = []string{publishedTip}
	case "merge":
		baseTip := resolvePromoteBaseTip(remoteTip, localTip, checkpoints)
		msg, _ := gitutil.CommitMessage(checkpoints[len(checkpoints)-1].SHA)
		msg = ensureChangeID(msg, changeID)
		publishedTip, err = promoteMerge(repoRoot, baseTip, checkpointSHAs[len(checkpointSHAs)-1], msg)
		if err != nil {
			return promoteResult{}, err
		}
		published = []string{publishedTip}
		mergeCommitSHA = &publishedTip
		mainlineVal := 1
		mainline = &mainlineVal
	default:
		return promoteResult{}, promoteError{
			Code:    "promote_invalid_strategy",
			Message: fmt.Sprintf("unsupported promote strategy %q", strategy),
		}
	}

	guardTip := strings.TrimSpace(remoteTip)
	if guardTip == "" {
		guardTip = localTip
	}
	if guardTip != "" && !opts.ForceTarget {
		if !gitutil.IsAncestor(guardTip, publishedTip) {
			return promoteResult{}, promoteError{
				Code:    "promote_non_fast_forward",
				Message: "promote would not be fast-forward; use --force-target to override",
				Next: []output.NextAction{
					{Action: "force", Command: fmt.Sprintf("jul promote --to %s --force-target", branch)},
				},
			}
		}
	}

	if remoteName != "" {
		if err := pushPublish(remoteName, publishedTip, branch, opts.ForceTarget); err != nil {
			return promoteResult{}, err
		}
	}
	if err := gitutil.UpdateRef(ref, publishedTip); err != nil {
		return promoteResult{}, err
	}

	eventID, err := recordPromoteMeta(promoteMetaInput{
		Branch:         branch,
		Strategy:       strategy,
		ChangeID:       changeID,
		AnchorSHA:      anchorSHA,
		Checkpoints:    checkpoints,
		CheckpointSHAs: checkpointSHAs,
		PublishedSHAs:  published,
		MergeCommitSHA: mergeCommitSHA,
		Mainline:       mainline,
	})
	if err != nil {
		return promoteResult{}, err
	}
	if err := writePromoteChangeIDNotes(changeID, eventID, strategy, checkpoints, published); err != nil {
		return promoteResult{}, err
	}

	trackTip = strings.TrimSpace(remoteTip)
	if trackTip == "" {
		trackTip = publishedTip
	}
	baseMarkerSHA, err := startNewDraftAfterPromote(repoRoot, sha, publishedTip, branch, trackTip)
	if err != nil {
		return promoteResult{}, err
	}

	return promoteResult{
		Branch:        branch,
		TargetSHA:     sha,
		Strategy:      strategy,
		Published:     published,
		PublishedTip:  publishedTip,
		BaseMarkerSHA: baseMarkerSHA,
		ForceTarget:   opts.ForceTarget,
		NoPolicy:      opts.NoPolicy,
	}, nil
}

type stackWorkspace struct {
	User string
	Name string
}

func promoteWithStack(opts promoteOptions) (promoteResult, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return promoteResult{}, err
	}
	user, workspace := workspaceParts()
	stack, baseBranch, err := resolvePromoteStack(repoRoot, user, workspace)
	if err != nil {
		return promoteResult{}, err
	}
	if len(stack) == 1 {
		return promoteLocal(opts)
	}
	if baseBranch != "" && strings.TrimSpace(opts.Branch) != strings.TrimSpace(baseBranch) {
		return promoteResult{}, fmt.Errorf("stacked workspace targets %s; use --to %s", baseBranch, baseBranch)
	}

	originalEnv := os.Getenv(config.EnvWorkspace)
	defer restoreWorkspaceEnv(originalEnv)

	var result promoteResult
	// Promote bottom-up (base workspace first).
	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		wsID := entry.User + "/" + entry.Name
		if err := withWorkspaceEnv(wsID); err != nil {
			return promoteResult{}, err
		}
		localOpts := opts
		if i != 0 {
			localOpts.TargetSHA = ""
		}
		res, err := promoteLocal(localOpts)
		if err != nil {
			return promoteResult{}, err
		}
		if i == 0 {
			result = res
		}
	}
	return result, nil
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

type promoteMetaInput struct {
	Branch         string
	Strategy       string
	ChangeID       string
	AnchorSHA      string
	Checkpoints    []metadata.ChangeCheckpoint
	CheckpointSHAs []string
	PublishedSHAs  []string
	MergeCommitSHA *string
	Mainline       *int
}

func recordPromoteMeta(input promoteMetaInput) (int, error) {
	if strings.TrimSpace(input.ChangeID) == "" {
		return 0, nil
	}
	anchorSHA := strings.TrimSpace(input.AnchorSHA)
	checkpoints := input.Checkpoints
	if len(checkpoints) == 0 {
		var err error
		anchorSHA, checkpoints, err = changeMetaFromCheckpoints(input.ChangeID)
		if err != nil {
			return 0, err
		}
	}
	if anchorSHA == "" {
		if len(checkpoints) > 0 {
			anchorSHA = checkpoints[0].SHA
		} else if len(input.PublishedSHAs) > 0 {
			anchorSHA = input.PublishedSHAs[0]
		}
	}
	if anchorSHA == "" {
		return 0, nil
	}

	meta, ok, err := metadata.ReadChangeMeta(anchorSHA)
	if err != nil {
		return 0, err
	}
	if !ok {
		meta = metadata.ChangeMeta{}
	}
	if meta.ChangeID == "" {
		meta.ChangeID = input.ChangeID
	}
	if meta.AnchorSHA == "" {
		meta.AnchorSHA = anchorSHA
	}
	if len(checkpoints) > 0 {
		meta.Checkpoints = checkpoints
	}
	eventID := len(meta.PromoteEvents) + 1
	strategy := strings.TrimSpace(input.Strategy)
	if strategy == "" {
		strategy = "rebase"
	}
	meta.PromoteEvents = append(meta.PromoteEvents, metadata.PromoteEvent{
		Target:         strings.TrimSpace(input.Branch),
		Strategy:       strategy,
		Timestamp:      time.Now().UTC(),
		Published:      input.PublishedSHAs,
		CheckpointSHAs: input.CheckpointSHAs,
		PublishedSHAs:  input.PublishedSHAs,
		MergeCommitSHA: input.MergeCommitSHA,
		Mainline:       input.Mainline,
	})
	return eventID, metadata.WriteChangeMeta(meta)
}

func startNewDraftAfterPromote(repoRoot, checkpointSHA, publishedSHA, branch, trackTip string) (string, error) {
	if strings.TrimSpace(publishedSHA) == "" {
		return "", nil
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return "", err
	}
	newChangeID, err := gitutil.NewChangeID()
	if err != nil {
		return "", err
	}
	treeSHA, err := gitutil.TreeOf(publishedSHA)
	if err != nil {
		return "", err
	}
	baseParent := strings.TrimSpace(checkpointSHA)
	if baseParent == "" {
		baseParent = strings.TrimSpace(publishedSHA)
	}
	baseMarkerSHA, err := gitutil.CreateWorkspaceBaseMarker(treeSHA, baseParent)
	if err != nil {
		return "", err
	}
	newDraftSHA, err := gitutil.CreateDraftCommitFromTree(treeSHA, baseMarkerSHA, newChangeID)
	if err != nil {
		return "", err
	}
	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)
	if err := gitutil.UpdateRef(syncRef, newDraftSHA); err != nil {
		return "", err
	}
	if err := gitutil.UpdateRef(workspaceRef, baseMarkerSHA); err != nil {
		return "", err
	}
	if err := writeWorkspaceLease(repoRoot, workspace, baseMarkerSHA); err != nil {
		return "", err
	}
	if err := gitutil.EnsureHeadRef(repoRoot, workspaceHeadRef(workspace), baseMarkerSHA); err != nil {
		return "", err
	}
	if strings.TrimSpace(branch) != "" {
		trackRef := "refs/heads/" + strings.TrimSpace(branch)
		if strings.TrimSpace(trackTip) == "" {
			trackTip = publishedSHA
		}
		_ = updateWorkspaceTracking(repoRoot, workspace, trackRef, strings.TrimSpace(trackTip))
	}
	if _, err := gitutil.Git("read-tree", "--reset", "-u", newDraftSHA); err != nil {
		return "", err
	}
	if _, err := gitutil.Git("clean", "-fd", "--exclude=.jul"); err != nil {
		return "", err
	}
	return baseMarkerSHA, nil
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
	remoteTip := strings.TrimSpace(fields[0])
	if err := fetchRef(remote.Name, ref); err != nil {
		return "", remote.Name, err
	}
	return remoteTip, remote.Name, nil
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
			fs, jsonOut := newFlagSet("changes")
			_ = fs.Parse(args)

			changes, err := localChanges()
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "changes_failed", fmt.Sprintf("failed to fetch changes: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to fetch changes: %v\n", err)
				}
				return 1
			}
			if *jsonOut {
				return writeJSON(changes)
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
				if hasJSONFlag(args) {
					_ = output.EncodeError(os.Stdout, "hooks_missing_subcommand", "missing hooks subcommand", nil)
					return 1
				}
				printHooksUsage()
				return 0
			}

			sub := strings.ToLower(args[0])
			switch sub {
			case "install":
				return runHooksInstall(args[1:])
			case "uninstall":
				return runHooksUninstall(args[1:])
			case "status":
				return runHooksStatus(args[1:])
			default:
				if hasJSONFlag(args) {
					_ = output.EncodeError(os.Stdout, "hooks_unknown_subcommand", fmt.Sprintf("unknown subcommand %q", sub), nil)
					return 1
				}
				printHooksUsage()
				return 1
			}
		},
	}
}

func printHooksUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul hooks <install|uninstall|status>")
}

type hooksOutput struct {
	Status    string `json:"status"`
	Action    string `json:"action,omitempty"`
	Installed bool   `json:"installed,omitempty"`
	Path      string `json:"path,omitempty"`
	Message   string `json:"message,omitempty"`
}

func runHooksInstall(args []string) int {
	fs, jsonOut := newFlagSet("hooks install")
	_ = fs.Parse(args)

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "hooks_repo_failed", fmt.Sprintf("failed to locate git repo: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to locate git repo: %v\n", err)
		}
		return 1
	}

	cliCmd := os.Getenv("JUL_HOOK_CMD")
	if cliCmd == "" {
		cliCmd = "jul"
	}
	path, err := hooks.InstallPostCommit(repoRoot, cliCmd)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "hooks_install_failed", fmt.Sprintf("install failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		}
		return 1
	}
	out := hooksOutput{
		Status:  "ok",
		Action:  "install",
		Path:    path,
		Message: fmt.Sprintf("installed post-commit hook: %s", path),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderHooksOutput(out)
	return 0
}

func runHooksUninstall(args []string) int {
	fs, jsonOut := newFlagSet("hooks uninstall")
	_ = fs.Parse(args)

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "hooks_repo_failed", fmt.Sprintf("failed to locate git repo: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to locate git repo: %v\n", err)
		}
		return 1
	}

	if err := hooks.UninstallPostCommit(repoRoot); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "hooks_uninstall_failed", fmt.Sprintf("uninstall failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
		}
		return 1
	}
	out := hooksOutput{
		Status:  "ok",
		Action:  "uninstall",
		Message: "removed post-commit hook",
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderHooksOutput(out)
	return 0
}

func runHooksStatus(args []string) int {
	fs, jsonOut := newFlagSet("hooks status")
	_ = fs.Parse(args)

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "hooks_repo_failed", fmt.Sprintf("failed to locate git repo: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to locate git repo: %v\n", err)
		}
		return 1
	}

	installed, path, err := hooks.StatusPostCommit(repoRoot)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "hooks_status_failed", fmt.Sprintf("status failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
		}
		return 1
	}
	out := hooksOutput{
		Status:    "ok",
		Action:    "status",
		Installed: installed,
		Path:      path,
	}
	if installed {
		out.Message = fmt.Sprintf("post-commit hook installed: %s", path)
	} else {
		out.Message = fmt.Sprintf("post-commit hook not installed (%s)", path)
	}
	if *jsonOut {
		if code := writeJSON(out); code != 0 {
			return code
		}
		if installed {
			return 0
		}
		return 1
	}
	renderHooksOutput(out)
	if installed {
		return 0
	}
	return 1
}

func renderHooksOutput(out hooksOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
	}
}

type versionOutput struct {
	Version string `json:"version"`
}

func newVersionCommand(version string) Command {
	return Command{
		Name:    "version",
		Summary: "Show CLI version",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("version")
			_ = fs.Parse(args)

			out := versionOutput{Version: version}
			if *jsonOut {
				return writeJSON(out)
			}
			renderVersionOutput(out)
			return 0
		},
	}
}

func renderVersionOutput(out versionOutput) {
	fmt.Fprintln(os.Stdout, out.Version)
}
