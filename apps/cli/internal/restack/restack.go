package restack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	wsconfig "github.com/lydakis/jul/cli/internal/workspace"
)

type Options struct {
	RepoRoot  string
	User      string
	Workspace string
	BaseRef   string
	BaseTip   string
	BaseSHA   string
}

type Result struct {
	NewDraftSHA    string
	NewParentSHA   string
	NewCheckpoints []string
	ChangeID       string
}

func Run(opts Options) (Result, error) {
	repoRoot := strings.TrimSpace(opts.RepoRoot)
	if repoRoot == "" {
		var err error
		repoRoot, err = gitutil.RepoTopLevel()
		if err != nil {
			return Result{}, err
		}
	}
	user := strings.TrimSpace(opts.User)
	workspace := strings.TrimSpace(opts.Workspace)
	if workspace == "" {
		workspace = "@"
	}
	if user == "" {
		user = strings.TrimSpace(config.UserNamespace())
		if user == "" {
			user = config.UserName()
		}
	}
	if user == "" {
		return Result{}, fmt.Errorf("user required for restack")
	}

	baseRef := strings.TrimSpace(opts.BaseRef)
	baseTip := strings.TrimSpace(opts.BaseTip)
	if baseRef == "" {
		return Result{}, fmt.Errorf("base ref required for restack")
	}
	if baseTip == "" {
		return Result{}, fmt.Errorf("base tip required for restack")
	}

	deviceID, err := config.DeviceID()
	if err != nil {
		return Result{}, err
	}

	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)

	draftSHA, err := gitutil.ResolveRef(syncRef)
	if err != nil {
		return Result{}, fmt.Errorf("failed to resolve draft: %w", err)
	}
	draftSHA = strings.TrimSpace(draftSHA)
	if draftSHA == "" {
		return Result{}, fmt.Errorf("draft sha required for restack")
	}

	msg, _ := gitutil.CommitMessage(draftSHA)
	changeID := gitutil.ExtractChangeID(msg)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(draftSHA)
	}
	if changeID == "" {
		return Result{}, fmt.Errorf("change id required for restack")
	}

	latest, err := latestCheckpointForChange(user, workspace, changeID)
	if err != nil {
		return Result{}, err
	}
	if latest == "" {
		return Result{}, fmt.Errorf("checkpoint required before restack")
	}

	chain, err := checkpointChain(latest, changeID)
	if err != nil {
		return Result{}, err
	}
	if len(chain) == 0 {
		return Result{}, fmt.Errorf("no checkpoints found for change")
	}

	worktree, err := agent.EnsureWorktree(repoRoot, baseTip, agent.WorktreeOptions{})
	if err != nil {
		return Result{}, err
	}

	newParent := baseTip
	prevTrace := ""
	lastAttested := ""
	newCheckpoints := make([]string, 0, len(chain))
	for idx, oldSHA := range chain {
		if att, _ := metadata.GetAttestation(oldSHA); att != nil {
			if strings.TrimSpace(att.Status) != "" {
				lastAttested = oldSHA
			} else if inheritFrom := strings.TrimSpace(att.AttestationInheritFrom); inheritFrom != "" {
				if inherited, _ := metadata.GetAttestation(inheritFrom); inherited != nil && strings.TrimSpace(inherited.Status) != "" {
					lastAttested = inheritFrom
				}
			}
		}
		if _, err := gitDir(worktree, nil, "cherry-pick", "--no-commit", oldSHA); err != nil {
			if isEmptyCherryPick(err) {
				_, _ = gitDir(worktree, nil, "cherry-pick", "--skip")
			} else {
				conflicts := restackConflictFiles(worktree)
				_, _ = gitDir(worktree, nil, "cherry-pick", "--abort")
				return Result{}, ConflictError{CheckpointSHA: oldSHA, Conflicts: conflicts}
			}
		}
		treeSHA, err := gitOutputDir(worktree, "write-tree")
		if err != nil {
			return Result{}, fmt.Errorf("failed to snapshot restack tree: %v", err)
		}

		oldMsg, _ := gitutil.CommitMessage(oldSHA)
		oldTraceBase := strings.TrimSpace(gitutil.ExtractTraceBase(oldMsg))
		oldTraceHead := strings.TrimSpace(gitutil.ExtractTraceHead(oldMsg))
		if idx == 0 && prevTrace == "" {
			prevTrace = oldTraceBase
		}

		traceParents := []string{}
		if strings.TrimSpace(prevTrace) != "" {
			traceParents = append(traceParents, strings.TrimSpace(prevTrace))
		}
		if strings.TrimSpace(oldTraceHead) != "" && strings.TrimSpace(oldTraceHead) != strings.TrimSpace(prevTrace) {
			traceParents = append(traceParents, strings.TrimSpace(oldTraceHead))
		}
		traceSHA, err := createRestackTrace(treeSHA, traceParents, deviceID)
		if err != nil {
			return Result{}, fmt.Errorf("failed to create restack trace: %v", err)
		}

		newMsg := stripTrailer(stripTrailer(oldMsg, "Trace-Head"), "Trace-Base")
		if prevTrace != "" {
			newMsg = addTrailer(newMsg, "Trace-Base", prevTrace)
		}
		if traceSHA != "" {
			newMsg = addTrailer(newMsg, "Trace-Head", traceSHA)
		}

		msgFile, err := os.CreateTemp("", "jul-restack-msg-")
		if err != nil {
			return Result{}, fmt.Errorf("failed to create message file: %v", err)
		}
		if _, err := msgFile.WriteString(newMsg); err != nil {
			_ = msgFile.Close()
			_ = os.Remove(msgFile.Name())
			return Result{}, fmt.Errorf("failed to write message file: %v", err)
		}
		_ = msgFile.Close()
		if _, err := gitDir(worktree, nil, "commit", "--no-verify", "--allow-empty", "-F", msgFile.Name()); err != nil {
			_ = os.Remove(msgFile.Name())
			return Result{}, fmt.Errorf("failed to create checkpoint: %v", err)
		}
		_ = os.Remove(msgFile.Name())

		newSHA, err := gitOutputDir(worktree, "rev-parse", "HEAD")
		if err != nil {
			return Result{}, fmt.Errorf("failed to resolve checkpoint: %v", err)
		}
		newSHA = strings.TrimSpace(newSHA)
		newParent = newSHA
		newCheckpoints = append(newCheckpoints, newSHA)
		prevTrace = traceSHA

		if lastAttested != "" {
			_ = metadata.WriteAttestationInheritance(newSHA, lastAttested)
		}

		keepRef := keepRefPrefix(user, workspace) + changeID + "/" + newSHA
		if err := gitutil.UpdateRef(keepRef, newSHA); err != nil {
			return Result{}, fmt.Errorf("failed to update keep-ref: %v", err)
		}
	}

	newDraft, err := gitutil.CreateDraftCommit(newParent, changeID)
	if err != nil {
		return Result{}, fmt.Errorf("failed to create new draft: %v", err)
	}

	if err := gitutil.UpdateRef(syncRef, newDraft); err != nil {
		return Result{}, err
	}
	if err := gitutil.UpdateRef(workspaceRef, newParent); err != nil {
		return Result{}, err
	}
	changeRef := fmt.Sprintf("refs/jul/changes/%s", changeID)
	if err := gitutil.UpdateRef(changeRef, newParent); err != nil {
		return Result{}, err
	}
	anchorRef := fmt.Sprintf("refs/jul/anchors/%s", changeID)
	if !gitutil.RefExists(anchorRef) && len(newCheckpoints) > 0 {
		if err := gitutil.UpdateRef(anchorRef, newCheckpoints[0]); err != nil {
			return Result{}, err
		}
	}
	if err := writeWorkspaceLease(repoRoot, workspace, newParent); err != nil {
		return Result{}, err
	}
	if err := ensureWorkspaceHead(repoRoot, workspace, newParent); err != nil {
		return Result{}, err
	}
	baseSHA := strings.TrimSpace(opts.BaseSHA)
	if baseSHA == "" {
		baseSHA = baseTip
	}
	trackRef := ""
	trackTip := ""
	if strings.HasPrefix(baseRef, "refs/heads/") {
		trackRef = baseRef
		trackTip = baseTip
	}
	if err := wsconfig.WriteConfig(repoRoot, workspace, wsconfig.Config{
		BaseRef:  baseRef,
		BaseSHA:  baseSHA,
		TrackRef: trackRef,
		TrackTip: trackTip,
	}); err != nil {
		return Result{}, err
	}
	if err := updateWorktree(repoRoot, newDraft); err != nil {
		return Result{}, err
	}

	return Result{
		NewDraftSHA:    newDraft,
		NewParentSHA:   newParent,
		NewCheckpoints: newCheckpoints,
		ChangeID:       changeID,
	}, nil
}

type keepRefInfo struct {
	Ref           string
	SHA           string
	ChangeID      string
	CheckpointSHA string
}

func listKeepRefs(prefix string) ([]keepRefInfo, error) {
	if strings.TrimSpace(prefix) == "" {
		return nil, fmt.Errorf("keep ref prefix required")
	}
	out, err := gitutil.Git("show-ref")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var refs []keepRefInfo
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sha := fields[0]
		ref := fields[1]
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		rest := strings.TrimPrefix(ref, prefix)
		parts := strings.Split(rest, "/")
		if len(parts) < 2 {
			continue
		}
		changeID := parts[0]
		checkpoint := parts[1]
		refs = append(refs, keepRefInfo{
			Ref:           ref,
			SHA:           sha,
			ChangeID:      changeID,
			CheckpointSHA: checkpoint,
		})
	}
	return refs, nil
}

func latestCheckpointForChange(user, workspace, changeID string) (string, error) {
	prefix := keepRefPrefix(user, workspace)
	refs, err := listKeepRefs(prefix)
	if err != nil {
		return "", err
	}
	type entry struct {
		sha  string
		when time.Time
	}
	entries := []entry{}
	for _, ref := range refs {
		if ref.ChangeID != changeID {
			continue
		}
		whenStr, _ := gitutil.Git("log", "-1", "--format=%cI", ref.CheckpointSHA)
		when, _ := time.Parse(time.RFC3339, strings.TrimSpace(whenStr))
		entries = append(entries, entry{sha: ref.CheckpointSHA, when: when})
	}
	if len(entries) == 0 {
		return "", nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].when.After(entries[j].when)
	})
	return entries[0].sha, nil
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
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

func createRestackTrace(treeSHA string, parents []string, deviceID string) (string, error) {
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

func updateWorktree(repoRoot, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("ref required for worktree update")
	}
	if _, err := gitutil.Git("-C", repoRoot, "read-tree", "--reset", "-u", ref); err != nil {
		return err
	}
	_, err := gitutil.Git("-C", repoRoot, "clean", "-fd", "--exclude=.jul")
	return err
}

func gitOutputDir(dir string, args ...string) (string, error) {
	out, err := gitDir(dir, nil, args...)
	return out, err
}

func gitDir(dir string, env map[string]string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), flattenEnv(env)...)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func flattenEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
}

func keepRefPrefix(user, workspace string) string {
	return fmt.Sprintf("refs/jul/keep/%s/%s/", user, workspace)
}

func writeWorkspaceLease(repoRoot, workspace, sha string) error {
	if strings.TrimSpace(sha) == "" {
		return fmt.Errorf("workspace lease sha required")
	}
	path := filepath.Join(repoRoot, ".jul", "workspaces", workspace, "lease")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sha+"\n"), 0o644)
}

func ensureWorkspaceHead(repoRoot, workspace, sha string) error {
	ref := fmt.Sprintf("refs/heads/jul/%s", workspace)
	return gitutil.EnsureHeadRef(repoRoot, ref, sha)
}

func isEmptyCherryPick(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "cherry-pick") {
		return false
	}
	if strings.Contains(msg, "previous cherry-pick is empty") {
		return true
	}
	if strings.Contains(msg, "patch is empty") {
		return true
	}
	if strings.Contains(msg, "nothing to commit") {
		return true
	}
	return false
}

type ConflictError struct {
	CheckpointSHA string
	Conflicts     []string
}

func (e ConflictError) Error() string {
	return "restack conflict"
}

func restackConflictFiles(worktree string) []string {
	out, err := gitDir(worktree, nil, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	conflicts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		conflicts = append(conflicts, line)
	}
	return conflicts
}
