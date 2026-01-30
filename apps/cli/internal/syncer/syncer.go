package syncer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

type Result struct {
	DraftSHA         string
	WorkspaceRef     string
	SyncRef          string
	WorkspaceUpdated bool
	BaseAdvanced     bool
	FastForwarded    bool
	RemoteName       string
	RemotePushed     bool
	Diverged         bool
	AutoMerged       bool
	RemoteProblem    string
}

type CheckpointResult struct {
	CheckpointSHA    string
	DraftSHA         string
	ChangeID         string
	TraceBase        string
	TraceHead        string
	WorkspaceRef     string
	SyncRef          string
	KeepRef          string
	WorkspaceUpdated bool
	RemoteName       string
	RemotePushed     bool
	Diverged         bool
	RemoteProblem    string
}

func Sync() (Result, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return Result{}, err
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return Result{}, err
	}

	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)

	res := Result{
		WorkspaceRef: workspaceRef,
		SyncRef:      syncRef,
	}

	workspaceTip := ""
	remote, rerr := remotesel.Resolve()
	if rerr == nil {
		res.RemoteName = remote.Name
	} else {
		switch rerr {
		case remotesel.ErrNoRemote:
			res.RemoteProblem = "no remote configured"
		case remotesel.ErrMultipleRemote:
			res.RemoteProblem = "multiple remotes found; run 'jul remote set <name>'"
		case remotesel.ErrRemoteMissing:
			res.RemoteProblem = "configured remote not found"
		default:
			return res, rerr
		}
	}

	if rerr == nil {
		_ = fetchRef(remote.Name, workspaceRef)
		if sha, err := gitutil.ResolveRef(workspaceRef); err == nil {
			workspaceTip = strings.TrimSpace(sha)
		}
	} else if gitutil.RefExists(workspaceRef) {
		if sha, err := gitutil.ResolveRef(workspaceRef); err == nil {
			workspaceTip = strings.TrimSpace(sha)
		}
	}

	baseSHA, _ := readWorkspaceLease(repoRoot, workspace)
	leaseBase := strings.TrimSpace(baseSHA)
	if leaseBase != "" {
		if msg, err := gitutil.CommitMessage(leaseBase); err == nil && isDraftMessage(msg) {
			if parent, err := gitutil.ParentOf(leaseBase); err == nil && strings.TrimSpace(parent) != "" {
				leaseBase = strings.TrimSpace(parent)
			}
		}
	}
	workspaceBase := strings.TrimSpace(workspaceTip)
	if workspaceBase != "" {
		if msg, err := gitutil.CommitMessage(workspaceBase); err == nil && isDraftMessage(msg) {
			if parent, err := gitutil.ParentOf(workspaceBase); err == nil && strings.TrimSpace(parent) != "" {
				workspaceBase = strings.TrimSpace(parent)
			}
		}
	}

	if leaseBase == "" && workspaceBase != "" {
		res.Diverged = true
		res.RemoteProblem = "workspace lease missing; run 'jul ws checkout' first"
	}
	if leaseBase != "" && workspaceBase != "" && leaseBase != workspaceBase {
		if !gitutil.IsAncestor(leaseBase, workspaceBase) {
			res.Diverged = true
			res.RemoteProblem = "workspace lease corrupted; run 'jul ws checkout' to realign"
		}
	}

	existingDraft := resolveExistingDraft(syncRef, workspaceRef)
	localBase := ""
	if existingDraft != "" {
		if msg, err := gitutil.CommitMessage(existingDraft); err == nil && isDraftMessage(msg) {
			if parent, err := gitutil.ParentOf(existingDraft); err == nil && strings.TrimSpace(parent) != "" {
				localBase = strings.TrimSpace(parent)
			}
		}
	}
	if localBase == "" {
		localBase = leaseBase
	}

	if !res.Diverged && workspaceTip != "" && localBase != "" && strings.TrimSpace(localBase) != strings.TrimSpace(workspaceTip) {
		res.BaseAdvanced = true
	}

	parentSHA, changeID := resolveDraftBase(workspaceRef, syncRef)
	if localBase != "" {
		parentSHA = localBase
	} else if leaseBase != "" {
		parentSHA = leaseBase
	}
	if strings.TrimSpace(parentSHA) == "" {
		if workspaceTip != "" {
			parentSHA = strings.TrimSpace(workspaceTip)
		} else if head, err := gitutil.ResolveRef("HEAD"); err == nil {
			parentSHA = strings.TrimSpace(head)
		}
	}
	if changeID == "" {
		if parentSHA != "" {
			if msg, err := gitutil.CommitMessage(parentSHA); err == nil {
				changeID = gitutil.ExtractChangeID(msg)
			}
		}
	}
	treeSHA, err := gitutil.DraftTree()
	if err != nil {
		return Result{}, err
	}

	if res.BaseAdvanced && !res.Diverged && workspaceTip != "" && parentSHA != "" {
		baseTree, err := gitutil.TreeOf(parentSHA)
		if err == nil && baseTree == treeSHA {
			if err := updateWorktree(repoRoot, workspaceTip); err != nil {
				return res, err
			}
			if err := writeWorkspaceLease(repoRoot, workspace, workspaceTip); err != nil {
				return res, err
			}
			res.FastForwarded = true
			res.BaseAdvanced = false
			parentSHA = strings.TrimSpace(workspaceTip)
			treeSHA, err = gitutil.DraftTree()
			if err != nil {
				return res, err
			}
		}
	}

	draftSHA, err := reuseOrCreateDraft(treeSHA, parentSHA, changeID, existingDraft)
	if err != nil {
		return Result{}, err
	}
	res.DraftSHA = draftSHA
	if err := gitutil.UpdateRef(syncRef, draftSHA); err != nil {
		return Result{}, err
	}

	if rerr == nil {
		if err := pushRef(remote.Name, draftSHA, syncRef, true); err != nil {
			return res, err
		}
		res.RemotePushed = true
	}

	return finalizeSync(res, !res.Diverged)
}

func finalizeSync(res Result, allowCanonical bool) (Result, error) {
	if _, err := Trace(TraceOptions{Implicit: true, UpdateCanonical: allowCanonical}); err != nil {
		return res, err
	}
	return res, nil
}

func ensureWorkspaceAligned(syncRes Result) error {
	if syncRes.Diverged || strings.Contains(syncRes.RemoteProblem, "base divergence") || strings.Contains(syncRes.RemoteProblem, "workspace lease missing") {
		if strings.TrimSpace(syncRes.RemoteProblem) != "" {
			return errors.New(syncRes.RemoteProblem)
		}
		return errors.New("workspace diverged; run 'jul merge' or 'jul ws checkout' to realign")
	}
	return nil
}

func Checkpoint(message string) (CheckpointResult, error) {
	traceRes, err := Trace(TraceOptions{Force: true, UpdateCanonical: true})
	if err != nil {
		return CheckpointResult{}, err
	}

	syncRes, err := Sync()
	if err != nil {
		return CheckpointResult{}, err
	}
	if err := ensureWorkspaceAligned(syncRes); err != nil {
		return CheckpointResult{}, err
	}

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return CheckpointResult{}, err
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return CheckpointResult{}, err
	}

	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)

	draftSHA := syncRes.DraftSHA
	if draftSHA == "" {
		return CheckpointResult{}, fmt.Errorf("draft sha missing")
	}

	draftMessage, _ := gitutil.CommitMessage(draftSHA)
	changeID := gitutil.ExtractChangeID(draftMessage)
	if changeID == "" {
		if generated, err := gitutil.NewChangeID(); err == nil {
			changeID = generated
		}
	}

	if strings.TrimSpace(message) == "" {
		message = "checkpoint"
	}
	message = ensureChangeID(message, changeID)
	if traceRes.CanonicalSHA != "" {
		message = ensureTrailer(message, "Trace-Head", traceRes.CanonicalSHA)
	}
	if traceRes.TraceBase != "" {
		message = ensureTrailer(message, "Trace-Base", traceRes.TraceBase)
	}

	treeSHA, err := gitutil.TreeOf(draftSHA)
	if err != nil {
		return CheckpointResult{}, err
	}
	parentSHA, _ := gitutil.ParentOf(draftSHA)
	checkpointSHA, err := gitutil.CommitTree(treeSHA, parentSHA, message)
	if err != nil {
		return CheckpointResult{}, err
	}

	keepRef := keepRefPath(user, workspace, changeID, checkpointSHA)
	if err := gitutil.UpdateRef(keepRef, checkpointSHA); err != nil {
		return CheckpointResult{}, err
	}

	newDraftSHA, err := gitutil.CreateDraftCommit(checkpointSHA, changeID)
	if err != nil {
		return CheckpointResult{}, err
	}
	if err := gitutil.UpdateRef(syncRef, newDraftSHA); err != nil {
		return CheckpointResult{}, err
	}

	res := CheckpointResult{
		CheckpointSHA: checkpointSHA,
		DraftSHA:      newDraftSHA,
		ChangeID:      changeID,
		TraceBase:     traceRes.TraceBase,
		TraceHead:     traceRes.CanonicalSHA,
		WorkspaceRef:  workspaceRef,
		SyncRef:       syncRef,
		KeepRef:       keepRef,
		RemoteName:    syncRes.RemoteName,
		RemotePushed:  syncRes.RemotePushed,
		Diverged:      syncRes.Diverged,
		RemoteProblem: syncRes.RemoteProblem,
	}

	if !syncRes.Diverged {
		if err := gitutil.UpdateRef(workspaceRef, checkpointSHA); err != nil {
			return res, err
		}
		res.WorkspaceUpdated = true
		if err := writeWorkspaceLease(repoRoot, workspace, checkpointSHA); err != nil {
			return res, err
		}
		if err := ensureWorkspaceHead(repoRoot, workspace, checkpointSHA); err != nil {
			return res, err
		}
	}

	if syncRes.RemoteName != "" {
		workspaceRemote := ""
		_ = fetchRef(syncRes.RemoteName, workspaceRef)
		if sha, err := gitutil.ResolveRef(workspaceRef); err == nil {
			workspaceRemote = sha
		}
		if err := pushRef(syncRes.RemoteName, newDraftSHA, syncRef, true); err != nil {
			return res, err
		}
		res.RemotePushed = true
		if res.WorkspaceUpdated {
			if err := pushWorkspace(syncRes.RemoteName, checkpointSHA, workspaceRef, workspaceRemote); err != nil {
				return res, err
			}
		}
		if err := pushRef(syncRes.RemoteName, checkpointSHA, keepRef, true); err != nil {
			return res, err
		}
	}

	return res, nil
}

func AdoptCheckpoint() (CheckpointResult, error) {
	traceRes, err := Trace(TraceOptions{Force: true, UpdateCanonical: true})
	if err != nil {
		return CheckpointResult{}, err
	}

	syncRes, err := Sync()
	if err != nil {
		return CheckpointResult{}, err
	}
	if err := ensureWorkspaceAligned(syncRes); err != nil {
		return CheckpointResult{}, err
	}

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return CheckpointResult{}, err
	}
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return CheckpointResult{}, err
	}

	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)

	headSHA, err := gitutil.Git("rev-parse", "HEAD")
	if err != nil {
		return CheckpointResult{}, err
	}
	headSHA = strings.TrimSpace(headSHA)
	if headSHA == "" {
		return CheckpointResult{}, fmt.Errorf("HEAD commit required")
	}
	headMsg, _ := gitutil.CommitMessage(headSHA)
	if isDraftMessage(headMsg) {
		return CheckpointResult{}, fmt.Errorf("cannot adopt draft commit")
	}
	changeID := gitutil.ExtractChangeID(headMsg)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(headSHA)
	}

	keepRef := keepRefPath(user, workspace, changeID, headSHA)
	if err := gitutil.UpdateRef(keepRef, headSHA); err != nil {
		return CheckpointResult{}, err
	}

	treeSHA, err := gitutil.DraftTree()
	if err != nil {
		return CheckpointResult{}, err
	}
	newDraftSHA, err := gitutil.CreateDraftCommitFromTree(treeSHA, headSHA, changeID)
	if err != nil {
		return CheckpointResult{}, err
	}
	if err := gitutil.UpdateRef(syncRef, newDraftSHA); err != nil {
		return CheckpointResult{}, err
	}

	res := CheckpointResult{
		CheckpointSHA: headSHA,
		DraftSHA:      newDraftSHA,
		ChangeID:      changeID,
		TraceBase:     traceRes.TraceBase,
		TraceHead:     traceRes.CanonicalSHA,
		WorkspaceRef:  workspaceRef,
		SyncRef:       syncRef,
		KeepRef:       keepRef,
		RemoteName:    syncRes.RemoteName,
		RemotePushed:  syncRes.RemotePushed,
		Diverged:      syncRes.Diverged,
		RemoteProblem: syncRes.RemoteProblem,
	}

	if !syncRes.Diverged {
		if err := gitutil.UpdateRef(workspaceRef, headSHA); err != nil {
			return res, err
		}
		res.WorkspaceUpdated = true
		if err := writeWorkspaceLease(repoRoot, workspace, headSHA); err != nil {
			return res, err
		}
		if err := ensureWorkspaceHead(repoRoot, workspace, headSHA); err != nil {
			return res, err
		}
	}

	if syncRes.RemoteName != "" {
		workspaceRemote := ""
		_ = fetchRef(syncRes.RemoteName, workspaceRef)
		if sha, err := gitutil.ResolveRef(workspaceRef); err == nil {
			workspaceRemote = sha
		}
		if err := pushRef(syncRes.RemoteName, newDraftSHA, syncRef, true); err != nil {
			return res, err
		}
		res.RemotePushed = true
		if res.WorkspaceUpdated {
			if err := pushWorkspace(syncRes.RemoteName, headSHA, workspaceRef, workspaceRemote); err != nil {
				return res, err
			}
		}
		if err := pushRef(syncRes.RemoteName, headSHA, keepRef, true); err != nil {
			return res, err
		}
	}

	return res, nil
}

func workspaceParts() (string, string) {
	id := strings.TrimSpace(config.WorkspaceID())
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	user := config.UserName()
	if user == "" {
		user = "user"
	}
	return user, id
}

func resolveDraftBase(workspaceRef, syncRef string) (string, string) {
	var baseRef string
	if gitutil.RefExists(syncRef) {
		baseRef = syncRef
	} else if gitutil.RefExists(workspaceRef) {
		baseRef = workspaceRef
	} else {
		baseRef = "HEAD"
	}

	var parentSHA string
	var changeID string
	var baseSHA string
	baseWasDraft := false
	if baseRef != "" {
		if sha, err := gitutil.ResolveRef(baseRef); err == nil {
			baseSHA = sha
			if msg, err := gitutil.CommitMessage(sha); err == nil {
				changeID = gitutil.ExtractChangeID(msg)
				if isDraftMessage(msg) {
					baseWasDraft = true
					if parent, err := gitutil.ParentOf(sha); err == nil {
						parentSHA = parent
					} else {
						parentSHA = sha
					}
				} else {
					parentSHA = sha
				}
			}
		}
	}
	if baseWasDraft && baseSHA != "" && parentSHA == baseSHA {
		if head, err := gitutil.ResolveRef("HEAD"); err == nil && strings.TrimSpace(head) != "" && head != parentSHA {
			if headMsg, err := gitutil.CommitMessage(head); err == nil && !isDraftMessage(headMsg) {
				parentSHA = head
			}
		}
	}
	if changeID == "" && parentSHA != "" {
		changeID = gitutil.FallbackChangeID(parentSHA)
	}
	if changeID == "" {
		if generated, err := gitutil.NewChangeID(); err == nil {
			changeID = generated
		}
	}
	return parentSHA, changeID
}

func resolveExistingDraft(syncRef, workspaceRef string) string {
	if gitutil.RefExists(syncRef) {
		if sha, err := gitutil.ResolveRef(syncRef); err == nil {
			return sha
		}
	}
	return ""
}

func reuseOrCreateDraft(treeSHA, parentSHA, changeID, existingDraft string) (string, error) {
	if existingDraft != "" {
		msg, err := gitutil.CommitMessage(existingDraft)
		if err == nil && isDraftMessage(msg) {
			parent, _ := gitutil.ParentOf(existingDraft)
			if strings.TrimSpace(parentSHA) == strings.TrimSpace(parent) {
				if baseTree, err := gitutil.TreeOf(existingDraft); err == nil && baseTree == treeSHA {
					return existingDraft, nil
				}
			}
		}
	}
	return gitutil.CreateDraftCommitFromTree(treeSHA, parentSHA, changeID)
}

func ensureChangeID(message, changeID string) string {
	if changeID == "" {
		return message
	}
	if gitutil.ExtractChangeID(message) != "" {
		return message
	}
	return strings.TrimSpace(message) + "\n\nChange-Id: " + changeID + "\n"
}

func ensureTrailer(message, key, value string) string {
	if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return message
	}
	if strings.TrimSpace(gitutil.ExtractTraceHead(message)) != "" && key == "Trace-Head" {
		return message
	}
	if strings.TrimSpace(gitutil.ExtractTraceBase(message)) != "" && key == "Trace-Base" {
		return message
	}
	return strings.TrimSpace(message) + "\n\n" + key + ": " + value + "\n"
}

func keepRefPath(user, workspace, changeID, checkpointSHA string) string {
	parts := []string{"refs/jul/keep"}
	if strings.TrimSpace(user) != "" {
		parts = append(parts, user)
	}
	if strings.TrimSpace(workspace) != "" {
		parts = append(parts, workspace)
	}
	if strings.TrimSpace(changeID) != "" {
		parts = append(parts, changeID)
	}
	if strings.TrimSpace(checkpointSHA) != "" {
		parts = append(parts, checkpointSHA)
	}
	return strings.Join(parts, "/")
}

func isDraftMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return strings.HasPrefix(trimmed, "[draft]")
}

func autoMerge(repoRoot, workspaceRemote, draftSHA, changeID string) (string, bool, error) {
	mergeBase, err := gitutil.MergeBase(workspaceRemote, draftSHA)
	if err != nil {
		return "", false, err
	}
	treeSHA, conflicts, err := mergeTree(repoRoot, mergeBase, workspaceRemote, draftSHA)
	if err != nil {
		return "", false, err
	}
	if conflicts {
		return "", false, nil
	}
	mergedSHA, err := gitutil.CreateDraftCommitFromTree(treeSHA, mergeBase, changeID)
	if err != nil {
		return "", false, err
	}
	return mergedSHA, true, nil
}

func mergeTree(repoRoot, baseSHA, theirsSHA, oursSHA string) (string, bool, error) {
	cmd := exec.Command("git", "-C", repoRoot, "merge-tree", "--write-tree", "--merge-base", baseSHA, oursSHA, theirsSHA)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(output))
	if err != nil {
		if strings.Contains(out, "CONFLICT") {
			return "", true, nil
		}
		return "", false, fmt.Errorf("git -C %s merge-tree --write-tree failed: %s", repoRoot, out)
	}
	treeSHA := out
	if treeSHA == "" {
		return "", false, fmt.Errorf("merge-tree returned empty tree")
	}
	return treeSHA, false, nil
}

func gitWithEnv(dir string, env map[string]string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), flattenEnv(env)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git -C %s %s failed: %s", dir, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func flattenEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
}

func updateWorktree(repoRoot, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("ref required for worktree update")
	}
	if _, err := gitWithEnv(repoRoot, nil, "read-tree", "--reset", "-u", ref); err != nil {
		return err
	}
	_, err := gitWithEnv(repoRoot, nil, "clean", "-fd", "--exclude=.jul")
	return err
}

func fetchRef(remoteName, ref string) error {
	if strings.TrimSpace(remoteName) == "" || strings.TrimSpace(ref) == "" {
		return nil
	}
	_, err := gitutil.Git("fetch", remoteName, "+"+ref+":"+ref)
	return err
}

func pushRef(remoteName, sha, ref string, force bool) error {
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	spec := fmt.Sprintf("%s:%s", sha, ref)
	args := []string{"push"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, remoteName, spec)
	_, err := gitutil.Git(args...)
	return err
}

func pushWorkspace(remoteName, sha, ref, old string) error {
	if strings.TrimSpace(remoteName) == "" {
		return nil
	}
	spec := fmt.Sprintf("%s:%s", sha, ref)
	args := []string{"push"}
	if strings.TrimSpace(old) != "" {
		args = append(args, "--force-with-lease="+ref+":"+old)
	}
	args = append(args, remoteName, spec)
	_, err := gitutil.Git(args...)
	return err
}

func readWorkspaceLease(repoRoot, workspace string) (string, error) {
	path := workspaceLeasePath(repoRoot, workspace)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeWorkspaceLease(repoRoot, workspace, sha string) error {
	if strings.TrimSpace(sha) == "" {
		return errors.New("workspace lease sha required")
	}
	path := workspaceLeasePath(repoRoot, workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sha+"\n"), 0o644)
}

func workspaceLeasePath(repoRoot, workspace string) string {
	return filepath.Join(repoRoot, ".jul", "workspaces", workspace, "lease")
}

func ensureWorkspaceHead(repoRoot, workspace, sha string) error {
	ref := fmt.Sprintf("refs/heads/jul/%s", workspace)
	return gitutil.EnsureHeadRef(repoRoot, ref, sha)
}
