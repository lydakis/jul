package syncer

import (
	"errors"
	"fmt"
	"os"
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

	parentSHA, changeID := resolveDraftBase(workspaceRef, syncRef)
	draftSHA, err := gitutil.CreateDraftCommit(parentSHA, changeID)
	if err != nil {
		return Result{}, err
	}
	if err := gitutil.UpdateRef(syncRef, draftSHA); err != nil {
		return Result{}, err
	}

	res := Result{
		DraftSHA:     draftSHA,
		WorkspaceRef: workspaceRef,
		SyncRef:      syncRef,
	}

	remote, rerr := remotesel.Resolve()
	if rerr == nil {
		res.RemoteName = remote.Name
		if err := pushRef(remote.Name, draftSHA, syncRef, true); err != nil {
			return res, err
		}
		res.RemotePushed = true
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

	workspaceRemote := ""
	if rerr == nil {
		_ = fetchRef(remote.Name, workspaceRef)
		if sha, err := gitutil.ResolveRef(workspaceRef); err == nil {
			workspaceRemote = sha
		}
	}

	baseSHA, _ := readWorkspaceBase(repoRoot, workspace)
	if baseSHA == "" && workspaceRemote != "" {
		res.Diverged = true
		res.RemoteProblem = "workspace baseline missing; run 'jul ws checkout' first"
		return res, nil
	}
	if workspaceRemote != "" && baseSHA != "" && workspaceRemote != baseSHA {
		res.Diverged = true
		return res, nil
	}

	if err := gitutil.UpdateRef(workspaceRef, draftSHA); err != nil {
		return res, err
	}
	res.WorkspaceUpdated = true
	if rerr == nil {
		if err := pushWorkspace(remote.Name, draftSHA, workspaceRef, workspaceRemote); err != nil {
			return res, err
		}
	}
	if err := writeWorkspaceBase(repoRoot, workspace, draftSHA); err != nil {
		return res, err
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
	if baseRef != "" {
		if sha, err := gitutil.ResolveRef(baseRef); err == nil {
			if msg, err := gitutil.CommitMessage(sha); err == nil {
				changeID = gitutil.ExtractChangeID(msg)
			}
			parentSHA = sha
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

func fetchRef(remoteName, ref string) error {
	if strings.TrimSpace(remoteName) == "" || strings.TrimSpace(ref) == "" {
		return nil
	}
	_, err := gitutil.Git("fetch", remoteName, ref+":"+ref)
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

func readWorkspaceBase(repoRoot, workspace string) (string, error) {
	path := workspaceBasePath(repoRoot, workspace)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeWorkspaceBase(repoRoot, workspace, sha string) error {
	if strings.TrimSpace(sha) == "" {
		return errors.New("workspace base sha required")
	}
	path := workspaceBasePath(repoRoot, workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sha+"\n"), 0o644)
}

func workspaceBasePath(repoRoot, workspace string) string {
	return filepath.Join(repoRoot, ".jul", "workspaces", workspace, "base")
}
