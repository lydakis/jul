package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/workspace"
)

func runWorkspaceRestack(args []string) int {
	fs := flag.NewFlagSet("ws restack", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	onto := fs.String("onto", "", "Retarget base to ref (e.g. main)")
	_ = fs.Parse(args)

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate repo root: %v\n", err)
		return 1
	}
	user, ws := workspaceParts()
	if ws == "" {
		ws = "@"
	}

	cfg, _, err := workspace.ReadConfig(repoRoot, ws)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read workspace config: %v\n", err)
		return 1
	}
	baseRef := strings.TrimSpace(cfg.BaseRef)
	if ontoVal := strings.TrimSpace(*onto); ontoVal != "" {
		baseRef, err = normalizeBaseRef(repoRoot, ontoVal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid --onto: %v\n", err)
			return 1
		}
	}
	if baseRef == "" {
		baseRef = detectBaseRef(repoRoot)
	}
	if baseRef == "" {
		fmt.Fprintln(os.Stderr, "base ref not found; use --onto to specify")
		return 1
	}

	baseTip, err := resolveBaseTip(repoRoot, baseRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve base ref: %v\n", err)
		return 1
	}
	if baseTip == "" {
		fmt.Fprintln(os.Stderr, "base tip not found; checkpoint required on base workspace")
		return 1
	}

	draftSHA, err := currentDraftSHA()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve draft: %v\n", err)
		return 1
	}
	msg, _ := gitutil.CommitMessage(draftSHA)
	changeID := gitutil.ExtractChangeID(msg)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(draftSHA)
	}

	latest, err := latestCheckpointForChange(changeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list checkpoints: %v\n", err)
		return 1
	}
	if latest == nil {
		fmt.Fprintln(os.Stderr, "checkpoint required before restack")
		return 1
	}

	chain, err := checkpointChain(latest.SHA, changeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build checkpoint chain: %v\n", err)
		return 1
	}
	if len(chain) == 0 {
		fmt.Fprintln(os.Stderr, "no checkpoints found for current change")
		return 1
	}

	deviceID, err := config.DeviceID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read device id: %v\n", err)
		return 1
	}

	newParent := baseTip
	prevTrace := ""
	newCheckpoints := make([]string, 0, len(chain))
	for idx, oldSHA := range chain {
		oldParent, _ := gitutil.ParentOf(oldSHA)
		oldParent = strings.TrimSpace(oldParent)
		if oldParent == "" {
			oldParent = newParent
		}
		treeSHA, conflicts, err := mergeTree(repoRoot, oldParent, newParent, oldSHA)
		if err != nil {
			fmt.Fprintf(os.Stderr, "restack failed: %v\n", err)
			return 1
		}
		if conflicts {
			fmt.Fprintln(os.Stderr, "restack conflict; run 'jul merge' to resolve")
			return 1
		}

		oldMsg, _ := gitutil.CommitMessage(oldSHA)
		oldTraceBase := strings.TrimSpace(gitutil.ExtractTraceBase(oldMsg))
		oldTraceHead := strings.TrimSpace(gitutil.ExtractTraceHead(oldMsg))
		if idx == 0 && prevTrace == "" {
			prevTrace = oldTraceBase
		}

		traceParent := oldTraceHead
		if traceParent == "" {
			traceParent = prevTrace
		}
		traceSHA, err := createRestackTrace(treeSHA, traceParent, deviceID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create restack trace: %v\n", err)
			return 1
		}

		newMsg := stripTrailer(stripTrailer(oldMsg, "Trace-Head"), "Trace-Base")
		if prevTrace != "" {
			newMsg = addTrailer(newMsg, "Trace-Base", prevTrace)
		}
		if traceSHA != "" {
			newMsg = addTrailer(newMsg, "Trace-Head", traceSHA)
		}

		newSHA, err := gitutil.CommitTree(treeSHA, newParent, newMsg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create checkpoint: %v\n", err)
			return 1
		}
		newParent = newSHA
		newCheckpoints = append(newCheckpoints, newSHA)
		prevTrace = traceSHA

		keepRef := keepRefPrefix(user, ws) + changeID + "/" + newSHA
		if err := gitutil.UpdateRef(keepRef, newSHA); err != nil {
			fmt.Fprintf(os.Stderr, "failed to update keep-ref: %v\n", err)
			return 1
		}
	}

	newDraft, err := gitutil.CreateDraftCommit(newParent, changeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create new draft: %v\n", err)
		return 1
	}

	workspaceRef := workspaceRef(user, ws)
	syncRef, err := syncRef(user, ws)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve sync ref: %v\n", err)
		return 1
	}
	if err := gitutil.UpdateRef(syncRef, newDraft); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update sync ref: %v\n", err)
		return 1
	}
	if err := gitutil.UpdateRef(workspaceRef, newDraft); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update workspace ref: %v\n", err)
		return 1
	}
	if err := writeWorkspaceLease(repoRoot, ws, newDraft); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update workspace lease: %v\n", err)
		return 1
	}
	if err := ensureWorkspaceConfig(repoRoot, ws, baseRef, baseTip); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update workspace config: %v\n", err)
		return 1
	}
	if err := updateWorktreeLocal(repoRoot, newDraft); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update working tree: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Restacked %d checkpoints onto %s\n", len(newCheckpoints), strings.TrimSpace(baseTip))
	return 0
}

func checkpointChain(latestSHA, changeID string) ([]string, error) {
	chain := []string{}
	sha := strings.TrimSpace(latestSHA)
	for sha != "" {
		chain = append(chain, sha)
		parent, err := gitutil.ParentOf(sha)
		if err != nil {
			break
		}
		parent = strings.TrimSpace(parent)
		if parent == "" {
			break
		}
		msg, err := gitutil.CommitMessage(parent)
		if err != nil {
			break
		}
		if gitutil.ExtractChangeID(msg) != changeID {
			break
		}
		sha = parent
	}
	// reverse to oldest -> newest
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

func normalizeBaseRef(repoRoot, value string) (string, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return "", fmt.Errorf("base ref required")
	}
	if strings.HasPrefix(val, "refs/") {
		return val, nil
	}
	ref := "refs/heads/" + val
	if refExists(repoRoot, ref) {
		return ref, nil
	}
	return ref, nil
}

func resolveBaseTip(repoRoot, baseRef string) (string, error) {
	if strings.TrimSpace(baseRef) == "" {
		return "", fmt.Errorf("base ref required")
	}
	sha, err := gitutil.Git("-C", repoRoot, "rev-parse", baseRef)
	if err != nil {
		return "", err
	}
	sha = strings.TrimSpace(sha)
	if strings.HasPrefix(baseRef, "refs/jul/workspaces/") {
		parent, err := gitutil.ParentOf(sha)
		if err == nil && strings.TrimSpace(parent) != "" {
			return strings.TrimSpace(parent), nil
		}
		return "", fmt.Errorf("base workspace has no checkpoint")
	}
	return sha, nil
}

func createRestackTrace(treeSHA, parentTrace, deviceID string) (string, error) {
	parents := []string{}
	if strings.TrimSpace(parentTrace) != "" {
		parents = append(parents, strings.TrimSpace(parentTrace))
	}
	traceSHA, err := gitutil.CommitTreeWithParents(treeSHA, parents, "[trace] restack")
	if err != nil {
		return "", err
	}
	note := metadata.TraceNote{
		TraceSHA:  traceSHA,
		TraceType: "restack",
		Agent:     "jul",
		Device:    strings.TrimSpace(deviceID),
		CreatedAt: time.Now().UTC(),
	}
	_ = metadata.WriteTrace(note)
	return traceSHA, nil
}

func stripTrailer(message, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return strings.TrimSpace(message)
	}
	lines := strings.Split(message, "\n")
	out := make([]string, 0, len(lines))
	prefix := key + ":"
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

func addTrailer(message, key, value string) string {
	if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return strings.TrimSpace(message)
	}
	return strings.TrimSpace(message) + "\n\n" + strings.TrimSpace(key) + ": " + strings.TrimSpace(value) + "\n"
}

func updateWorktreeLocal(repoRoot, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("ref required for worktree update")
	}
	if _, err := gitutil.Git("-C", repoRoot, "read-tree", "--reset", "-u", ref); err != nil {
		return err
	}
	_, err := gitutil.Git("-C", repoRoot, "clean", "-fd", "--exclude=.jul")
	return err
}

func mergeTree(repoRoot, baseSHA, oursSHA, theirsSHA string) (string, bool, error) {
	args := []string{"-C", repoRoot, "merge-tree", "--write-tree", "--merge-base", baseSHA, oursSHA, theirsSHA}
	out, err := gitutil.Git(args...)
	out = strings.TrimSpace(out)
	if err != nil {
		if strings.Contains(out, "CONFLICT") {
			return "", true, nil
		}
		return "", false, fmt.Errorf("git merge-tree failed: %v", err)
	}
	if out == "" {
		return "", false, fmt.Errorf("merge-tree returned empty tree")
	}
	return out, false, nil
}
