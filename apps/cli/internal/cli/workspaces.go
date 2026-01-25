package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func newWorkspaceCommand() Command {
	return Command{
		Name:    "ws",
		Summary: "Manage workspaces",
		Run: func(args []string) int {
			if len(args) == 0 {
				fmt.Fprintln(os.Stdout, config.WorkspaceID())
				return 0
			}

			sub := args[0]
			switch sub {
			case "list":
				return runWorkspaceList(args[1:])
			case "checkout":
				return runWorkspaceCheckout(args[1:])
			case "set":
				return runWorkspaceSet(args[1:])
			case "new":
				return runWorkspaceNew(args[1:])
			case "switch":
				return runWorkspaceSwitch(args[1:])
			case "stack":
				return runWorkspaceStack(args[1:])
			case "rename":
				return runWorkspaceRename(args[1:])
			case "delete":
				return runWorkspaceDelete(args[1:])
			case "current":
				fmt.Fprintln(os.Stdout, config.WorkspaceID())
				return 0
			default:
				printWorkspaceUsage()
				return 1
			}
		},
	}
}

func runWorkspaceList(args []string) int {
	fs := flag.NewFlagSet("ws list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.Parse(args)
	workspaces, err := localWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list workspaces: %v\n", err)
		return 1
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stdout, "No workspaces.")
		return 0
	}
	for _, ws := range workspaces {
		fmt.Fprintf(os.Stdout, "%s %s %s\n", ws.WorkspaceID, ws.Repo, ws.Branch)
	}
	return 0
}

func runWorkspaceSet(args []string) int {
	fs := flag.NewFlagSet("ws set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	user := fs.String("user", "", "Override user for workspace id")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "workspace name required")
		return 1
	}

	wsID, _, _, err := resolveWorkspaceID(name, *user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	if err := runGitConfig("jul.workspace", wsID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Workspace set to %s\n", wsID)
	return 0
}

func runWorkspaceNew(args []string) int {
	fs := flag.NewFlagSet("ws new", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	user := fs.String("user", "", "Override user for workspace id")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "workspace name required")
		return 1
	}
	wsID, wsUser, wsName, err := resolveWorkspaceID(name, *user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	_, currentWorkspace := workspaceParts()
	if err := saveWorkspaceState(currentWorkspace); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save workspace: %v\n", err)
		return 1
	}
	if _, err := syncer.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to sync current workspace: %v\n", err)
		return 1
	}

	baseSHA, err := currentBaseSHA()
	if err != nil {
		if head, err := gitutil.Git("rev-parse", "HEAD"); err == nil {
			baseSHA = strings.TrimSpace(head)
		}
	}
	treeSHA, err := gitutil.DraftTree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to snapshot working tree: %v\n", err)
		return 1
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate repo root: %v\n", err)
		return 1
	}
	baseRef := detectBaseRef(repoRoot)
	newDraftSHA, err := createWorkspaceDraft(wsUser, wsName, baseRef, baseSHA, treeSHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create workspace: %v\n", err)
		return 1
	}
	if err := runGitConfig("jul.workspace", wsID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Created workspace '%s'\n", wsName)
	fmt.Fprintf(os.Stdout, "Draft %s started.\n", strings.TrimSpace(newDraftSHA))
	return 0
}

func runWorkspaceSwitch(args []string) int {
	fs := flag.NewFlagSet("ws switch", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "workspace name required")
		return 1
	}
	currentUser, currentWorkspace := workspaceParts()
	if err := saveWorkspaceState(currentWorkspace); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save workspace: %v\n", err)
		return 1
	}
	if _, err := syncer.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to sync current workspace: %v\n", err)
		return 1
	}

	wsID, wsUser, wsName, err := resolveWorkspaceID(name, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if err := switchToWorkspace(wsUser, wsName); err != nil {
		fmt.Fprintf(os.Stderr, "switch failed: %v\n", err)
		return 1
	}
	if err := runGitConfig("jul.workspace", wsID); err != nil {
		rollbackErr := switchToWorkspaceLocal(currentUser, currentWorkspace)
		if rollbackErr != nil {
			fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
			fmt.Fprintf(os.Stderr, "switch rollback failed: %v\n", rollbackErr)
			fmt.Fprintf(os.Stderr, "workspace is now '%s' but config remains '%s'\n", wsName, config.WorkspaceID())
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		fmt.Fprintln(os.Stderr, "switch rolled back")
		return 1
	}
	fmt.Fprintf(os.Stdout, "Switched to workspace '%s'\n", wsName)
	return 0
}

func runWorkspaceStack(args []string) int {
	fs := flag.NewFlagSet("ws stack", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	user := fs.String("user", "", "Override user for workspace id")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "workspace name required")
		return 1
	}
	wsID, wsUser, wsName, err := resolveWorkspaceID(name, *user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	currentUser, currentName := workspaceParts()
	if err := saveWorkspaceState(currentName); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save workspace: %v\n", err)
		return 1
	}
	if _, err := syncer.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to sync current workspace: %v\n", err)
		return 1
	}

	checkpoint, err := latestCheckpoint()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read checkpoints: %v\n", err)
		return 1
	}
	if checkpoint == nil {
		fmt.Fprintln(os.Stderr, "checkpoint required before stacking; run 'jul checkpoint' first")
		return 1
	}
	treeSHA, err := gitutil.TreeOf(checkpoint.SHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read checkpoint tree: %v\n", err)
		return 1
	}
	parentRef := workspaceRef(currentUser, currentName)
	newDraftSHA, err := createWorkspaceDraft(wsUser, wsName, parentRef, checkpoint.SHA, treeSHA)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create stacked workspace: %v\n", err)
		return 1
	}
	if err := resetToDraft(newDraftSHA); err != nil {
		fmt.Fprintf(os.Stderr, "failed to reset to new workspace: %v\n", err)
		return 1
	}
	if err := runGitConfig("jul.workspace", wsID); err != nil {
		rollbackErr := switchToWorkspaceLocal(currentUser, currentName)
		if rollbackErr != nil {
			fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
			fmt.Fprintf(os.Stderr, "switch rollback failed: %v\n", rollbackErr)
			fmt.Fprintf(os.Stderr, "workspace is now '%s' but config remains '%s'\n", wsName, config.WorkspaceID())
			return 1
		}
		fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		fmt.Fprintln(os.Stderr, "switch rolled back")
		return 1
	}
	fmt.Fprintf(os.Stdout, "Created workspace '%s' (stacked on %s)\n", wsName, currentName)
	fmt.Fprintf(os.Stdout, "Draft %s started.\n", strings.TrimSpace(newDraftSHA))
	return 0
}

func runWorkspaceCheckout(args []string) int {
	fs := flag.NewFlagSet("ws checkout", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "workspace name required")
		return 1
	}

	user, _ := workspaceParts()
	targetName := name
	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) == 2 {
			user = parts[0]
			targetName = parts[1]
		}
	}
	if user == "" {
		user = config.UserName()
	}
	if user == "" {
		fmt.Fprintln(os.Stderr, "workspace user required")
		return 1
	}
	wsID := user + "/" + targetName

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to locate repo: %v\n", err)
		return 1
	}

	remote, rerr := remotesel.Resolve()
	if rerr == nil {
		if err := ensureJulRefspecs(repoRoot, remote.Name); err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure remote: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "Fetching workspace '%s'...\n", targetName)
		if _, err := gitutil.Git("-C", repoRoot, "fetch", remote.Name); err != nil {
			fmt.Fprintf(os.Stderr, "fetch failed: %v\n", err)
			return 1
		}
	} else if rerr != remotesel.ErrNoRemote {
		fmt.Fprintf(os.Stderr, "remote resolution failed: %v\n", rerr)
		return 1
	} else {
		fmt.Fprintf(os.Stdout, "Checking out workspace '%s'...\n", targetName)
	}

	ref := workspaceRef(user, targetName)
	if !gitutil.RefExists(ref) {
		fmt.Fprintf(os.Stderr, "workspace ref not found: %s\n", ref)
		fmt.Fprintln(os.Stderr, "Run 'jul sync' or create a workspace first.")
		return 1
	}
	sha, err := gitutil.ResolveRef(ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve workspace ref: %v\n", err)
		return 1
	}

	if _, err := gitutil.Git("-C", repoRoot, "read-tree", "--reset", "-u", sha); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update working tree: %v\n", err)
		return 1
	}
	if _, err := gitutil.Git("-C", repoRoot, "clean", "-fd", "--exclude=.jul"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to clean working tree: %v\n", err)
		return 1
	}

	syncRef, err := syncRef(user, targetName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve sync ref: %v\n", err)
		return 1
	}
	if err := gitutil.UpdateRef(syncRef, sha); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update sync ref: %v\n", err)
		return 1
	}
	if err := writeWorkspaceLease(repoRoot, targetName, sha); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update workspace lease: %v\n", err)
		return 1
	}

	if err := runGitConfig("jul.workspace", wsID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "  ✓ Workspace ref: %s\n", strings.TrimSpace(sha))
	fmt.Fprintln(os.Stdout, "  ✓ Working tree updated")
	fmt.Fprintln(os.Stdout, "  ✓ Sync ref initialized")
	fmt.Fprintln(os.Stdout, "  ✓ workspace_lease set")
	fmt.Fprintf(os.Stdout, "Switched to workspace '%s'\n", targetName)
	return 0
}

func runWorkspaceRename(args []string) int {
	fs := flag.NewFlagSet("ws rename", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.Parse(args)
	newName := strings.TrimSpace(fs.Arg(0))
	if newName == "" {
		fmt.Fprintln(os.Stderr, "new workspace name required")
		return 1
	}
	current := config.WorkspaceID()
	parts := strings.SplitN(current, "/", 2)
	user := parts[0]
	newID := newName
	if !strings.Contains(newName, "/") {
		newID = user + "/" + newName
	}
	if err := runGitConfig("jul.workspace", newID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to rename workspace: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Workspace renamed to %s\n", newID)
	return 0
}

func runWorkspaceDelete(args []string) int {
	fs := flag.NewFlagSet("ws delete", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "workspace name required")
		return 1
	}
	current := config.WorkspaceID()
	target := name
	if !strings.Contains(name, "/") {
		parts := strings.SplitN(current, "/", 2)
		user := parts[0]
		target = user + "/" + name
	}
	if target == current {
		fmt.Fprintln(os.Stderr, "cannot delete current workspace")
		return 1
	}
	if err := deleteWorkspaceLocal(target); err != nil {
		fmt.Fprintf(os.Stderr, "failed to delete workspace: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Deleted workspace %s\n", target)
	return 0
}

func resolveWorkspaceID(name, userOverride string) (string, string, string, error) {
	if strings.TrimSpace(name) == "" {
		return "", "", "", fmt.Errorf("workspace name required")
	}
	if strings.Contains(name, "/") {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", "", fmt.Errorf("invalid workspace name")
		}
		return name, parts[0], parts[1], nil
	}
	owner := strings.TrimSpace(userOverride)
	if owner == "" {
		owner = config.ServerUser()
	}
	if owner == "" {
		owner = strings.Split(config.WorkspaceID(), "/")[0]
	}
	if owner == "" {
		return "", "", "", fmt.Errorf("workspace user required")
	}
	return owner + "/" + name, owner, name, nil
}

func workspaceNameOnly() string {
	_, workspace := workspaceParts()
	if workspace == "" {
		return "@"
	}
	return workspace
}

func saveWorkspaceState(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if err := localDelete(name); err == nil {
		// removed existing snapshot
	}
	_, err := localSave(name)
	return err
}

func switchToWorkspace(user, workspace string) error {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	remote, rerr := remotesel.Resolve()
	if rerr == nil {
		if err := ensureJulRefspecs(repoRoot, remote.Name); err != nil {
			return err
		}
		if _, err := gitutil.Git("-C", repoRoot, "fetch", remote.Name); err != nil {
			return err
		}
	} else if rerr != remotesel.ErrNoRemote && rerr != remotesel.ErrMultipleRemote && rerr != remotesel.ErrRemoteMissing {
		return rerr
	}

	return switchToWorkspaceLocal(user, workspace)
}

func switchToWorkspaceLocal(user, workspace string) error {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	ref := workspaceRef(user, workspace)
	if !gitutil.RefExists(ref) {
		return fmt.Errorf("workspace ref not found: %s", ref)
	}
	sha, err := gitutil.ResolveRef(ref)
	if err != nil {
		return err
	}
	if err := resetToDraft(sha); err != nil {
		return err
	}
	syncRef, err := syncRef(user, workspace)
	if err != nil {
		return err
	}
	if err := gitutil.UpdateRef(syncRef, sha); err != nil {
		return err
	}
	if err := writeWorkspaceLease(repoRoot, workspace, sha); err != nil {
		return err
	}
	if err := localRestore(workspace); err != nil {
		if !strings.Contains(err.Error(), "local state not found") {
			return err
		}
	}
	return nil
}

func createWorkspaceDraft(user, workspace, baseRef, baseSHA, treeSHA string) (string, error) {
	if strings.TrimSpace(baseSHA) == "" {
		return "", fmt.Errorf("base commit required")
	}
	if strings.TrimSpace(treeSHA) == "" {
		return "", fmt.Errorf("tree sha required")
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return "", err
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		return "", err
	}
	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, workspace)
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, workspace)
	if gitutil.RefExists(workspaceRef) {
		return "", fmt.Errorf("workspace already exists: %s/%s", user, workspace)
	}
	if gitutil.RefExists(syncRef) {
		return "", fmt.Errorf("workspace already exists: %s/%s", user, workspace)
	}
	changeID, err := gitutil.NewChangeID()
	if err != nil {
		return "", err
	}
	draftSHA, err := gitutil.CreateDraftCommitFromTree(treeSHA, baseSHA, changeID)
	if err != nil {
		return "", err
	}
	if err := gitutil.UpdateRef(syncRef, draftSHA); err != nil {
		return "", err
	}
	if err := gitutil.UpdateRef(workspaceRef, draftSHA); err != nil {
		return "", err
	}
	if err := writeWorkspaceLease(repoRoot, workspace, draftSHA); err != nil {
		return "", err
	}
	if err := ensureWorkspaceConfig(repoRoot, workspace, baseRef, baseSHA); err != nil {
		return "", err
	}
	return draftSHA, nil
}

func resetToDraft(draftSHA string) error {
	if strings.TrimSpace(draftSHA) == "" {
		return fmt.Errorf("draft sha required")
	}
	if _, err := gitutil.Git("reset", "--hard", draftSHA); err != nil {
		return err
	}
	if _, err := gitutil.Git("clean", "-fd", "--exclude=.jul"); err != nil {
		return err
	}
	return nil
}

func localWorkspaces() ([]client.Workspace, error) {
	userParts := strings.SplitN(config.WorkspaceID(), "/", 2)
	if len(userParts) < 2 {
		return nil, nil
	}
	user := userParts[0]
	refsOut, err := gitutil.Git("show-ref")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(refsOut), "\n")
	seen := map[string]client.Workspace{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sha := fields[0]
		ref := fields[1]
		prefix := "refs/jul/workspaces/" + user + "/"
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		name := strings.TrimPrefix(ref, prefix)
		if name == "" {
			continue
		}
		wsID := user + "/" + name
		seen[wsID] = client.Workspace{
			WorkspaceID:   wsID,
			Repo:          config.RepoName(),
			Branch:        name,
			LastCommitSHA: sha,
			LastChangeID:  "",
		}
	}
	workspaces := make([]client.Workspace, 0, len(seen))
	for _, ws := range seen {
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}

func deleteWorkspaceLocal(target string) error {
	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("workspace id must be user/name")
	}
	user := parts[0]
	name := parts[1]
	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", user, name)
	if gitutil.RefExists(workspaceRef) {
		if _, err := gitutil.Git("update-ref", "-d", workspaceRef); err != nil {
			return err
		}
	}
	if deviceID, err := config.DeviceID(); err == nil {
		syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", user, deviceID, name)
		if gitutil.RefExists(syncRef) {
			_, _ = gitutil.Git("update-ref", "-d", syncRef)
		}
	}
	if root, err := gitutil.RepoTopLevel(); err == nil {
		leasePath := filepath.Join(root, ".jul", "workspaces", name, "lease")
		_ = os.Remove(leasePath)
	}
	return nil
}

func writeWorkspaceLease(repoRoot, workspace, sha string) error {
	if strings.TrimSpace(sha) == "" {
		return fmt.Errorf("workspace lease sha required")
	}
	path := filepath.Join(repoRoot, ".jul", "workspaces", workspace, "lease")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(sha)+"\n"), 0o644)
}

func runGitConfig(key, value string) error {
	cmd := exec.Command("git", "config", key, value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git config %s: %s", key, strings.TrimSpace(string(output)))
	}
	return nil
}

func printWorkspaceUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul ws [list|checkout|set|new|stack|switch|rename|delete|current]")
}
