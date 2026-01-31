package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

func newDraftCommand() Command {
	return Command{
		Name:    "draft",
		Summary: "Manage per-device drafts",
		Run: func(args []string) int {
			if len(args) == 0 {
				return runDraftList(args)
			}
			switch args[0] {
			case "list":
				return runDraftList(args[1:])
			case "show":
				return runDraftShow(args[1:])
			case "adopt":
				return runDraftAdopt(args[1:])
			default:
				fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", args[0])
				return 2
			}
		},
	}
}

func runDraftList(args []string) int {
	fs := flag.NewFlagSet("draft list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	remote := fs.Bool("remote", false, "List drafts from other devices")
	jsonOut := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)

	var drafts []output.DraftInfo
	var err error
	if *remote {
		drafts, err = listRemoteDrafts()
	} else {
		drafts, err = listLocalDrafts()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "draft list failed: %v\n", err)
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(output.DraftList{Drafts: drafts}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		return 0
	}

	output.RenderDraftList(os.Stdout, drafts, output.DefaultOptions())
	return 0
}

func runDraftShow(args []string) int {
	fs := flag.NewFlagSet("draft show", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	remote := fs.Bool("remote", false, "Show remote draft")
	jsonOut := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)
	device := strings.TrimSpace(fs.Arg(0))
	if device == "" {
		fmt.Fprintln(os.Stderr, "device required")
		return 2
	}

	info, err := draftInfoForDevice(device, *remote)
	if err != nil {
		fmt.Fprintf(os.Stderr, "draft show failed: %v\n", err)
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(output.DraftShow{Draft: info}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		return 0
	}
	output.RenderDraftShow(os.Stdout, info, output.DefaultOptions())
	return 0
}

func runDraftAdopt(args []string) int {
	fs := flag.NewFlagSet("draft adopt", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	onto := fs.String("onto", "", "Adopt onto a specific checkpoint")
	replace := fs.Bool("replace", false, "Discard local draft and take remote as-is")
	jsonOut := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)
	device := strings.TrimSpace(fs.Arg(0))
	if device == "" {
		fmt.Fprintln(os.Stderr, "device required")
		return 2
	}

	res, err := adoptDraft(draftAdoptOptions{
		Device:  device,
		Onto:    strings.TrimSpace(*onto),
		Replace: *replace,
	})
	if err != nil {
		var conflict MergeConflictError
		if errors.As(err, &conflict) {
			if strings.TrimSpace(conflict.Reason) != "" {
				fmt.Fprintln(os.Stderr, conflict.Reason)
			}
			if strings.TrimSpace(conflict.Worktree) != "" {
				fmt.Fprintf(os.Stderr, "Resolve conflicts in %s and rerun 'jul draft adopt %s'.\n", conflict.Worktree, device)
			}
			return 1
		}
		fmt.Fprintf(os.Stderr, "draft adopt failed: %v\n", err)
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

	fmt.Fprintf(os.Stdout, "Adopting draft from %s...\n", res.Device)
	if res.BaseSHA != "" {
		fmt.Fprintf(os.Stdout, "  ✓ Base %s\n", shortDraftID(res.BaseSHA, 6))
	}
	if res.Replaced {
		fmt.Fprintln(os.Stdout, "  ✓ Draft replaced")
	} else {
		fmt.Fprintln(os.Stdout, "  ✓ Draft merged")
	}
	fmt.Fprintln(os.Stdout, "  ✓ Working tree updated")
	return 0
}

type draftAdoptOptions struct {
	Device  string
	Onto    string
	Replace bool
}

type draftAdoptResult struct {
	Device      string `json:"device"`
	BaseSHA     string `json:"base_sha,omitempty"`
	DraftSHA    string `json:"draft_sha"`
	Replaced    bool   `json:"replaced,omitempty"`
	FilesMerged int    `json:"files_merged,omitempty"`
}

func adoptDraft(opts draftAdoptOptions) (draftAdoptResult, error) {
	device := strings.TrimSpace(opts.Device)
	if device == "" {
		return draftAdoptResult{}, fmt.Errorf("device required")
	}
	if !config.DraftSyncEnabled() {
		return draftAdoptResult{}, fmt.Errorf("draft sync unavailable; run 'jul doctor'")
	}

	remote, err := remotesel.Resolve()
	if err != nil {
		return draftAdoptResult{}, err
	}

	localDraft, localBase, err := currentDraftAndBase()
	if err != nil {
		return draftAdoptResult{}, err
	}
	if strings.TrimSpace(localDraft) == "" {
		return draftAdoptResult{}, fmt.Errorf("local draft not found; run 'jul sync' first")
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}

	remoteRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, device, workspace)
	remoteDraft, err := remoteDraftSHA(remote.Name, remoteRef)
	if err != nil {
		return draftAdoptResult{}, err
	}
	if strings.TrimSpace(remoteDraft) == "" {
		return draftAdoptResult{}, fmt.Errorf("remote draft not found for %s", device)
	}
	if err := fetchRemoteRef(remote.Name, remoteRef); err != nil {
		return draftAdoptResult{}, err
	}

	remoteBase := ""
	if parent, err := gitutil.ParentOf(remoteDraft); err == nil {
		remoteBase = strings.TrimSpace(parent)
	}
	if remoteBase == "" {
		remoteBase = strings.TrimSpace(remoteDraft)
	}

	baseSHA := strings.TrimSpace(localBase)
	if opts.Onto != "" {
		baseSHA, err = resolveRefSHA(opts.Onto)
		if err != nil {
			return draftAdoptResult{}, err
		}
	}

	if opts.Onto == "" && strings.TrimSpace(remoteBase) != baseSHA {
		return draftAdoptResult{}, fmt.Errorf("base mismatch: local %s, remote %s (use --onto to override)", shortDraftID(baseSHA, 6), shortDraftID(remoteBase, 6))
	}

	changeID := changeIDForCommit(localDraft)
	if changeID == "" {
		changeID = changeIDForCommit(remoteDraft)
	}

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return draftAdoptResult{}, err
	}

	ours := strings.TrimSpace(localDraft)
	theirs := strings.TrimSpace(remoteDraft)
	if opts.Onto != "" {
		ours, err = rebaseDraftOnto(repoRoot, baseSHA, localDraft, changeID)
		if err != nil {
			return draftAdoptResult{}, err
		}
		theirs, err = rebaseDraftOnto(repoRoot, baseSHA, remoteDraft, changeID)
		if err != nil {
			return draftAdoptResult{}, err
		}
	}

	newDraft := ""
	if opts.Replace {
		newDraft = theirs
	} else {
		newDraft, err = mergeDrafts(repoRoot, baseSHA, ours, theirs, changeID)
		if err != nil {
			return draftAdoptResult{}, err
		}
	}

	user, ws := workspaceParts()
	if ws == "" {
		ws = "@"
	}
	syncRef, err := syncRef(user, ws)
	if err != nil {
		return draftAdoptResult{}, err
	}
	if err := gitutil.UpdateRef(syncRef, newDraft); err != nil {
		return draftAdoptResult{}, err
	}
	if err := updateWorktreeLocal(repoRoot, newDraft); err != nil {
		return draftAdoptResult{}, err
	}
	if config.DraftSyncEnabled() && strings.TrimSpace(remote.Name) != "" {
		if err := pushRef(remote.Name, newDraft, syncRef, true); err != nil {
			return draftAdoptResult{}, err
		}
	}

	return draftAdoptResult{
		Device:   device,
		BaseSHA:  baseSHA,
		DraftSHA: newDraft,
		Replaced: opts.Replace,
	}, nil
}

func listLocalDrafts() ([]output.DraftInfo, error) {
	_, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	draftSHA, baseSHA, err := currentDraftAndBase()
	if err != nil {
		return nil, err
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return nil, err
	}
	info, err := draftInfoFromSHA(deviceID, workspace, draftSHA, baseSHA)
	if err != nil {
		return nil, err
	}
	return []output.DraftInfo{info}, nil
}

func listRemoteDrafts() ([]output.DraftInfo, error) {
	if !config.DraftSyncEnabled() {
		return nil, nil
	}
	remote, err := remotesel.Resolve()
	if err != nil {
		if errors.Is(err, remotesel.ErrNoRemote) || errors.Is(err, remotesel.ErrRemoteMissing) {
			return nil, nil
		}
		return nil, err
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	pattern := fmt.Sprintf("refs/jul/sync/%s/*/%s", user, workspace)
	out, err := gitutil.Git("ls-remote", remote.Name, pattern)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	drafts := make([]output.DraftInfo, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sha := strings.TrimSpace(fields[0])
		ref := strings.TrimSpace(fields[1])
		device := parseDraftDevice(ref, user, workspace)
		if device == "" {
			continue
		}
		if err := fetchRemoteRef(remote.Name, ref); err != nil {
			return nil, err
		}
		base := ""
		if parent, err := gitutil.ParentOf(sha); err == nil {
			base = strings.TrimSpace(parent)
		}
		if base == "" {
			base = sha
		}
		info, err := draftInfoFromSHA(device, workspace, sha, base)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, info)
	}
	return drafts, nil
}

func draftInfoForDevice(device string, remote bool) (output.DraftInfo, error) {
	device = strings.TrimSpace(device)
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	currentDevice, err := config.DeviceID()
	if err != nil {
		return output.DraftInfo{}, err
	}

	if device == currentDevice && !remote {
		draftSHA, baseSHA, err := currentDraftAndBase()
		if err != nil {
			return output.DraftInfo{}, err
		}
		return draftInfoFromSHA(device, workspace, draftSHA, baseSHA)
	}

	if !config.DraftSyncEnabled() {
		return output.DraftInfo{}, fmt.Errorf("draft sync unavailable; run 'jul doctor'")
	}

	remoteSel, err := remotesel.Resolve()
	if err != nil {
		return output.DraftInfo{}, err
	}
	ref := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, device, workspace)
	sha, err := remoteDraftSHA(remoteSel.Name, ref)
	if err != nil {
		return output.DraftInfo{}, err
	}
	if strings.TrimSpace(sha) == "" {
		return output.DraftInfo{}, fmt.Errorf("draft not found for %s", device)
	}
	if err := fetchRemoteRef(remoteSel.Name, ref); err != nil {
		return output.DraftInfo{}, err
	}
	base := ""
	if parent, err := gitutil.ParentOf(sha); err == nil {
		base = strings.TrimSpace(parent)
	}
	if base == "" {
		base = sha
	}
	return draftInfoFromSHA(device, workspace, sha, base)
}

func draftInfoFromSHA(device, workspace, draftSHA, baseSHA string) (output.DraftInfo, error) {
	updated := time.Time{}
	if out, err := gitutil.Git("show", "-s", "--format=%cI", strings.TrimSpace(draftSHA)); err == nil {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(out)); err == nil {
			updated = parsed
		}
	}
	msg, _ := gitutil.CommitMessage(draftSHA)
	changeID := gitutil.ExtractChangeID(msg)
	filesChanged := 0
	if strings.TrimSpace(baseSHA) != "" && strings.TrimSpace(draftSHA) != "" {
		if files, err := diffNameOnly(baseSHA, draftSHA); err == nil {
			filesChanged = len(files)
		}
	}
	return output.DraftInfo{
		Device:       device,
		Workspace:    workspace,
		DraftSHA:     strings.TrimSpace(draftSHA),
		BaseSHA:      strings.TrimSpace(baseSHA),
		ChangeID:     changeID,
		UpdatedAt:    updated,
		FilesChanged: filesChanged,
	}, nil
}

func parseDraftDevice(ref, user, workspace string) string {
	trimmed := strings.TrimSpace(ref)
	if !strings.HasPrefix(trimmed, "refs/jul/sync/") {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 6 {
		return ""
	}
	if parts[3] != user || parts[5] != workspace {
		return ""
	}
	return parts[4]
}

func remoteDraftSHA(remoteName, ref string) (string, error) {
	out, err := gitutil.Git("ls-remote", remoteName, ref)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.TrimSpace(fields[0]), nil
}

func fetchRemoteRef(remoteName, ref string) error {
	if strings.TrimSpace(remoteName) == "" || strings.TrimSpace(ref) == "" {
		return nil
	}
	_, err := gitutil.Git("fetch", remoteName, ref)
	return err
}

func resolveRefSHA(ref string) (string, error) {
	sha, err := gitutil.Git("rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func rebaseDraftOnto(repoRoot, baseSHA, draftSHA, changeID string) (string, error) {
	worktree, err := agent.EnsureWorktree(repoRoot, baseSHA, agent.WorktreeOptions{})
	if err != nil {
		return "", err
	}
	if err := gitDir(worktree, nil, "cherry-pick", "--no-commit", draftSHA); err != nil {
		conflicts := mergeConflictFiles(worktree)
		_ = gitDir(worktree, nil, "cherry-pick", "--abort")
		if len(conflicts) > 0 {
			return "", MergeConflictError{Worktree: worktree, Conflicts: conflicts, Reason: "conflicts while restacking draft"}
		}
		return "", err
	}
	treeSHA, err := gitOutputDir(worktree, "write-tree")
	if err != nil {
		return "", err
	}
	_ = gitDir(worktree, nil, "reset", "--hard", baseSHA)
	_ = gitDir(worktree, nil, "clean", "-fd")
	return gitutil.CreateDraftCommitFromTree(treeSHA, baseSHA, changeID)
}

func mergeDrafts(repoRoot, baseSHA, oursSHA, theirsSHA, changeID string) (string, error) {
	worktree, err := agent.EnsureWorktree(repoRoot, oursSHA, agent.WorktreeOptions{})
	if err != nil {
		return "", err
	}
	if err := gitDir(worktree, nil, "reset", "--hard", oursSHA); err != nil {
		return "", err
	}
	if err := gitDir(worktree, nil, "clean", "-fd"); err != nil {
		return "", err
	}
	mergeOutput, mergeErr := gitOutputDirAllowErr(worktree, "merge", "--no-commit", "--no-ff", theirsSHA)
	conflicts := mergeConflictFiles(worktree)
	if mergeErr != nil && len(conflicts) == 0 {
		return "", fmt.Errorf("merge failed: %s", strings.TrimSpace(mergeOutput))
	}

	if len(conflicts) > 0 {
		provider, err := agent.ResolveProvider()
		if err != nil {
			if errors.Is(err, agent.ErrAgentNotConfigured) || errors.Is(err, agent.ErrBundledMissing) {
				return "", MergeConflictError{
					Worktree:  worktree,
					Conflicts: conflicts,
					Reason:    "Agent not available; resolve conflicts manually.",
				}
			}
			return "", err
		}
		diff, _ := gitOutputDir(worktree, "diff")
		files := mergeConflictDetails(worktree, conflicts)
		req := agent.ReviewRequest{
			Version:       1,
			Action:        "resolve_conflict",
			WorkspacePath: worktree,
			Context: agent.ReviewContext{
				Checkpoint: baseSHA,
				ChangeID:   changeID,
				Diff:       diff,
				Files:      files,
				Conflicts:  conflicts,
			},
		}
		if _, err := agent.RunReview(context.Background(), provider, req); err != nil {
			return "", err
		}
	}

	if err := gitDir(worktree, nil, "add", "-A"); err != nil {
		return "", err
	}
	if unresolved := mergeConflictFiles(worktree); len(unresolved) > 0 {
		return "", MergeConflictError{Worktree: worktree, Conflicts: unresolved}
	}
	treeSHA, err := gitOutputDir(worktree, "write-tree")
	if err != nil {
		return "", err
	}
	return gitutil.CreateDraftCommitFromTree(treeSHA, baseSHA, changeID)
}

func shortDraftID(value string, n int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if n <= 0 {
		n = 6
	}
	if len(trimmed) <= n {
		return trimmed
	}
	return trimmed[:n] + "..."
}
