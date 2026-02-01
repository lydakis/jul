package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
	"github.com/lydakis/jul/cli/internal/syncer"
	wsconfig "github.com/lydakis/jul/cli/internal/workspace"
)

type workspaceActionOutput struct {
	Status       string `json:"status"`
	Action       string `json:"action"`
	Workspace    string `json:"workspace_id"`
	Message      string `json:"message,omitempty"`
	DraftSHA     string `json:"draft_sha,omitempty"`
	Parent       string `json:"parent_workspace,omitempty"`
	WorkspaceRef string `json:"workspace_ref,omitempty"`
	SyncRef      string `json:"sync_ref,omitempty"`
	WorkspaceSHA string `json:"workspace_sha,omitempty"`
}

type workspaceListOutput struct {
	Workspaces []client.Workspace `json:"workspaces"`
}

func newWorkspaceCommand() Command {
	return Command{
		Name:    "ws",
		Summary: "Manage workspaces",
		Run: func(args []string) int {
			jsonOut, args := stripJSONFlag(args)
			if len(args) == 0 {
				if jsonOut {
					args = ensureJSONFlag(args)
				}
				return runWorkspaceCurrent(args)
			}

			sub := args[0]
			subArgs := args[1:]
			if jsonOut {
				subArgs = ensureJSONFlag(subArgs)
			}
			if sub == "current" {
				return runWorkspaceCurrent(subArgs)
			}
			switch sub {
			case "list":
				return runWorkspaceList(subArgs)
			case "checkout":
				return runWorkspaceCheckout(subArgs)
			case "set":
				return runWorkspaceSet(subArgs)
			case "new":
				return runWorkspaceNew(subArgs)
			case "switch":
				return runWorkspaceSwitch(subArgs)
			case "stack":
				return runWorkspaceStack(subArgs)
			case "restack":
				return runWorkspaceRestack(subArgs)
			case "rename":
				return runWorkspaceRename(subArgs)
			case "delete":
				return runWorkspaceDelete(subArgs)
			default:
				if jsonOut {
					_ = output.EncodeError(os.Stdout, "workspace_unknown_subcommand", fmt.Sprintf("unknown subcommand %q", sub), nil)
					return 1
				}
				printWorkspaceUsage()
				return 1
			}
		},
	}
}

func runWorkspaceCurrent(args []string) int {
	fs, jsonOut := newFlagSet("ws")
	_ = fs.Parse(args)

	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "current",
		Workspace: config.WorkspaceID(),
		Message:   config.WorkspaceID(),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceAction(out)
	return 0
}

func runWorkspaceList(args []string) int {
	fs, jsonOut := newFlagSet("ws list")
	_ = fs.Parse(args)
	workspaces, err := localWorkspaces()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_list_failed", fmt.Sprintf("failed to list workspaces: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to list workspaces: %v\n", err)
		}
		return 1
	}
	out := workspaceListOutput{Workspaces: workspaces}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceList(out)
	return 0
}

func runWorkspaceSet(args []string) int {
	fs, jsonOut := newFlagSet("ws set")
	user := fs.String("user", "", "Override user for workspace id")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_name", "workspace name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "workspace name required")
		}
		return 1
	}

	wsID, _, _, err := resolveWorkspaceID(name, *user)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_resolve_failed", err.Error(), nil)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return 1
	}

	if err := runGitConfig("jul.workspace", wsID); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_set_failed", fmt.Sprintf("failed to set workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		}
		return 1
	}
	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "set",
		Workspace: wsID,
		Message:   fmt.Sprintf("Workspace set to %s", wsID),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceAction(out)
	return 0
}

func runWorkspaceNew(args []string) int {
	fs, jsonOut := newFlagSet("ws new")
	user := fs.String("user", "", "Override user for workspace id")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_name", "workspace name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "workspace name required")
		}
		return 1
	}
	wsID, wsUser, wsName, err := resolveWorkspaceID(name, *user)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_resolve_failed", err.Error(), nil)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return 1
	}
	_, currentWorkspace := workspaceParts()
	if err := saveWorkspaceState(currentWorkspace); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_save_failed", fmt.Sprintf("failed to save workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to save workspace: %v\n", err)
		}
		return 1
	}
	if _, err := syncer.Sync(); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_sync_failed", fmt.Sprintf("failed to sync current workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to sync current workspace: %v\n", err)
		}
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
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_snapshot_failed", fmt.Sprintf("failed to snapshot working tree: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to snapshot working tree: %v\n", err)
		}
		return 1
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_repo_failed", fmt.Sprintf("failed to locate repo root: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to locate repo root: %v\n", err)
		}
		return 1
	}
	baseRef := detectBaseRef(repoRoot)
	newDraftSHA, err := createWorkspaceDraft(wsUser, wsName, baseRef, baseSHA, treeSHA)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_create_failed", fmt.Sprintf("failed to create workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to create workspace: %v\n", err)
		}
		return 1
	}
	if err := runGitConfig("jul.workspace", wsID); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_set_failed", fmt.Sprintf("failed to set workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		}
		return 1
	}
	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "new",
		Workspace: wsID,
		DraftSHA:  strings.TrimSpace(newDraftSHA),
		Message:   fmt.Sprintf("Created workspace '%s'", wsName),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceAction(out)
	return 0
}

func runWorkspaceSwitch(args []string) int {
	fs, jsonOut := newFlagSet("ws switch")
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_name", "workspace name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "workspace name required")
		}
		return 1
	}
	currentUser, currentWorkspace := workspaceParts()
	if err := saveWorkspaceState(currentWorkspace); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_save_failed", fmt.Sprintf("failed to save workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to save workspace: %v\n", err)
		}
		return 1
	}
	if _, err := syncer.Sync(); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_sync_failed", fmt.Sprintf("failed to sync current workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to sync current workspace: %v\n", err)
		}
		return 1
	}

	wsID, wsUser, wsName, err := resolveWorkspaceID(name, "")
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_resolve_failed", err.Error(), nil)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return 1
	}
	if err := switchToWorkspace(wsUser, wsName); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_switch_failed", fmt.Sprintf("switch failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "switch failed: %v\n", err)
		}
		return 1
	}
	if err := runGitConfig("jul.workspace", wsID); err != nil {
		rollbackErr := switchToWorkspaceLocal(currentUser, currentWorkspace)
		if rollbackErr != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "workspace_set_failed", fmt.Sprintf("failed to set workspace: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
				fmt.Fprintf(os.Stderr, "switch rollback failed: %v\n", rollbackErr)
				fmt.Fprintf(os.Stderr, "workspace is now '%s' but config remains '%s'\n", wsName, config.WorkspaceID())
			}
			return 1
		}
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_set_failed", fmt.Sprintf("failed to set workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
			fmt.Fprintln(os.Stderr, "switch rolled back")
		}
		return 1
	}
	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "switch",
		Workspace: wsID,
		Message:   fmt.Sprintf("Switched to workspace '%s'", wsName),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceAction(out)
	return 0
}

func runWorkspaceStack(args []string) int {
	fs, jsonOut := newFlagSet("ws stack")
	user := fs.String("user", "", "Override user for workspace id")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_name", "workspace name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "workspace name required")
		}
		return 1
	}
	wsID, wsUser, wsName, err := resolveWorkspaceID(name, *user)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_resolve_failed", err.Error(), nil)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return 1
	}

	currentUser, currentName := workspaceParts()
	if err := saveWorkspaceState(currentName); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_save_failed", fmt.Sprintf("failed to save workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to save workspace: %v\n", err)
		}
		return 1
	}
	if _, err := syncer.Sync(); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_sync_failed", fmt.Sprintf("failed to sync current workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to sync current workspace: %v\n", err)
		}
		return 1
	}

	checkpoint, err := latestCheckpoint()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_checkpoint_failed", fmt.Sprintf("failed to read checkpoints: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to read checkpoints: %v\n", err)
		}
		return 1
	}
	if checkpoint == nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_checkpoint_required", "checkpoint required before stacking; run 'jul checkpoint' first", nil)
		} else {
			fmt.Fprintln(os.Stderr, "checkpoint required before stacking; run 'jul checkpoint' first")
		}
		return 1
	}
	treeSHA, err := gitutil.TreeOf(checkpoint.SHA)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_checkpoint_tree_failed", fmt.Sprintf("failed to read checkpoint tree: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to read checkpoint tree: %v\n", err)
		}
		return 1
	}
	parentRef := changeRef(checkpoint.ChangeID)
	newDraftSHA, err := createWorkspaceDraft(wsUser, wsName, parentRef, checkpoint.SHA, treeSHA)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_create_failed", fmt.Sprintf("failed to create stacked workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to create stacked workspace: %v\n", err)
		}
		return 1
	}
	if err := resetToDraft(newDraftSHA); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_reset_failed", fmt.Sprintf("failed to reset to new workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to reset to new workspace: %v\n", err)
		}
		return 1
	}
	if err := runGitConfig("jul.workspace", wsID); err != nil {
		rollbackErr := switchToWorkspaceLocal(currentUser, currentName)
		if rollbackErr != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "workspace_set_failed", fmt.Sprintf("failed to set workspace: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
				fmt.Fprintf(os.Stderr, "switch rollback failed: %v\n", rollbackErr)
				fmt.Fprintf(os.Stderr, "workspace is now '%s' but config remains '%s'\n", wsName, config.WorkspaceID())
			}
			return 1
		}
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_set_failed", fmt.Sprintf("failed to set workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
			fmt.Fprintln(os.Stderr, "switch rolled back")
		}
		return 1
	}
	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "stack",
		Workspace: wsID,
		Parent:    currentUser + "/" + currentName,
		DraftSHA:  strings.TrimSpace(newDraftSHA),
		Message:   fmt.Sprintf("Created workspace '%s' (stacked on %s)", wsName, currentName),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceAction(out)
	return 0
}

func runWorkspaceCheckout(args []string) int {
	fs, jsonOut := newFlagSet("ws checkout")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_name", "workspace name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "workspace name required")
		}
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
		user = strings.TrimSpace(config.UserNamespace())
		if user == "" {
			user = config.UserName()
		}
	}
	if user == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_user", "workspace user required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "workspace user required")
		}
		return 1
	}
	wsID := user + "/" + targetName

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_repo_failed", fmt.Sprintf("failed to locate repo: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to locate repo: %v\n", err)
		}
		return 1
	}

	remote, rerr := remotesel.Resolve()
	if rerr == nil {
		if err := ensureJulRefspecs(repoRoot, remote.Name); err != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "workspace_remote_failed", fmt.Sprintf("failed to configure remote: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "failed to configure remote: %v\n", err)
			}
			return 1
		}
		if _, err := gitutil.Git("-C", repoRoot, "fetch", remote.Name); err != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "workspace_fetch_failed", fmt.Sprintf("fetch failed: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "fetch failed: %v\n", err)
			}
			return 1
		}
	} else if rerr != remotesel.ErrNoRemote {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_remote_failed", fmt.Sprintf("remote resolution failed: %v", rerr), nil)
		} else {
			fmt.Fprintf(os.Stderr, "remote resolution failed: %v\n", rerr)
		}
		return 1
	}

	ref := workspaceRef(user, targetName)
	if !gitutil.RefExists(ref) {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_ref_missing", fmt.Sprintf("workspace ref not found: %s", ref), nil)
		} else {
			fmt.Fprintf(os.Stderr, "workspace ref not found: %s\n", ref)
			fmt.Fprintln(os.Stderr, "Run 'jul sync' or create a workspace first.")
		}
		return 1
	}
	sha, err := gitutil.ResolveRef(ref)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_ref_failed", fmt.Sprintf("failed to resolve workspace ref: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to resolve workspace ref: %v\n", err)
		}
		return 1
	}

	if _, err := gitutil.Git("-C", repoRoot, "read-tree", "--reset", "-u", sha); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_read_tree_failed", fmt.Sprintf("failed to update working tree: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to update working tree: %v\n", err)
		}
		return 1
	}
	if _, err := gitutil.Git("-C", repoRoot, "clean", "-fd", "--exclude=.jul"); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_clean_failed", fmt.Sprintf("failed to clean working tree: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to clean working tree: %v\n", err)
		}
		return 1
	}

	syncRef, err := syncRef(user, targetName)
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_sync_ref_failed", fmt.Sprintf("failed to resolve sync ref: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to resolve sync ref: %v\n", err)
		}
		return 1
	}
	if err := gitutil.UpdateRef(syncRef, sha); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_sync_ref_failed", fmt.Sprintf("failed to update sync ref: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to update sync ref: %v\n", err)
		}
		return 1
	}
	if err := writeWorkspaceLease(repoRoot, targetName, sha); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_lease_failed", fmt.Sprintf("failed to update workspace lease: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to update workspace lease: %v\n", err)
		}
		return 1
	}
	if err := gitutil.EnsureHeadRef(repoRoot, workspaceHeadRef(targetName), sha); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_head_failed", fmt.Sprintf("failed to update workspace head: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to update workspace head: %v\n", err)
		}
		return 1
	}
	refreshWorkspaceTrackTip(repoRoot, targetName)

	if err := runGitConfig("jul.workspace", wsID); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_set_failed", fmt.Sprintf("failed to set workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		}
		return 1
	}

	out := workspaceActionOutput{
		Status:       "ok",
		Action:       "checkout",
		Workspace:    wsID,
		WorkspaceRef: ref,
		WorkspaceSHA: strings.TrimSpace(sha),
		SyncRef:      syncRef,
		Message:      fmt.Sprintf("Switched to workspace '%s'", targetName),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceCheckout(out)
	return 0
}

func runWorkspaceRename(args []string) int {
	fs, jsonOut := newFlagSet("ws rename")
	_ = fs.Parse(args)
	newName := strings.TrimSpace(fs.Arg(0))
	if newName == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_name", "new workspace name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "new workspace name required")
		}
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
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_rename_failed", fmt.Sprintf("failed to rename workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to rename workspace: %v\n", err)
		}
		return 1
	}
	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "rename",
		Workspace: newID,
		Message:   fmt.Sprintf("Workspace renamed to %s", newID),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceAction(out)
	return 0
}

func runWorkspaceDelete(args []string) int {
	fs, jsonOut := newFlagSet("ws delete")
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_missing_name", "workspace name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "workspace name required")
		}
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
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_delete_current", "cannot delete current workspace", nil)
		} else {
			fmt.Fprintln(os.Stderr, "cannot delete current workspace")
		}
		return 1
	}
	if err := deleteWorkspaceLocal(target); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "workspace_delete_failed", fmt.Sprintf("failed to delete workspace: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to delete workspace: %v\n", err)
		}
		return 1
	}
	out := workspaceActionOutput{
		Status:    "ok",
		Action:    "delete",
		Workspace: target,
		Message:   fmt.Sprintf("Deleted workspace %s", target),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderWorkspaceAction(out)
	return 0
}

func renderWorkspaceAction(out workspaceActionOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
	}
	if out.DraftSHA != "" {
		fmt.Fprintf(os.Stdout, "Draft %s started.\n", out.DraftSHA)
	}
}

func renderWorkspaceList(out workspaceListOutput) {
	if len(out.Workspaces) == 0 {
		fmt.Fprintln(os.Stdout, "No workspaces.")
		return
	}
	for _, ws := range out.Workspaces {
		fmt.Fprintf(os.Stdout, "%s %s %s\n", ws.WorkspaceID, ws.Repo, ws.Branch)
	}
}

func renderWorkspaceCheckout(out workspaceActionOutput) {
	if out.WorkspaceSHA != "" {
		fmt.Fprintf(os.Stdout, "  ✓ Workspace ref: %s\n", out.WorkspaceSHA)
	}
	fmt.Fprintln(os.Stdout, "  ✓ Working tree updated")
	if out.SyncRef != "" {
		fmt.Fprintln(os.Stdout, "  ✓ Sync ref initialized")
	}
	fmt.Fprintln(os.Stdout, "  ✓ workspace_lease set")
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
	}
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
		if cfg, ok, err := wsconfig.ReadConfig(repoRoot, workspace); err == nil && ok {
			baseSHA := strings.TrimSpace(cfg.BaseSHA)
			if baseSHA == "" {
				if head, err := gitutil.Git("-C", repoRoot, "rev-parse", "HEAD"); err == nil {
					baseSHA = strings.TrimSpace(head)
				}
			}
			if baseSHA == "" {
				if workspace == "@" {
					if head, err := gitutil.Git("-C", repoRoot, "rev-parse", "HEAD"); err == nil && strings.TrimSpace(head) != "" {
						baseSHA = strings.TrimSpace(head)
					}
				}
				if baseSHA == "" {
					return fmt.Errorf("workspace ref not found: %s", ref)
				}
			}
			if err := gitutil.UpdateRef(ref, baseSHA); err != nil {
				return fmt.Errorf("workspace ref not found: %s", ref)
			}
		} else {
			if workspace == "@" {
				if head, err := gitutil.Git("-C", repoRoot, "rev-parse", "HEAD"); err == nil && strings.TrimSpace(head) != "" {
					if err := gitutil.UpdateRef(ref, strings.TrimSpace(head)); err == nil {
						// ref restored from HEAD
					} else {
						return fmt.Errorf("workspace ref not found: %s", ref)
					}
				} else {
					return fmt.Errorf("workspace ref not found: %s", ref)
				}
			} else {
				return fmt.Errorf("workspace ref not found: %s", ref)
			}
		}
	}
	sha, err := gitutil.ResolveRef(ref)
	if err != nil {
		return err
	}
	if err := resetToDraft(sha); err != nil {
		return err
	}
	if err := localRestore(workspace); err != nil {
		if !strings.Contains(err.Error(), "local state not found") {
			return err
		}
	}
	syncRef, err := syncRef(user, workspace)
	if err != nil {
		return err
	}
	changeID := ""
	if msg, err := gitutil.CommitMessage(sha); err == nil {
		changeID = gitutil.ExtractChangeID(msg)
	}
	if changeID == "" {
		if generated, err := gitutil.NewChangeID(); err == nil {
			changeID = generated
		}
	}
	treeSHA, err := gitutil.DraftTree()
	if err != nil {
		return err
	}
	draftSHA, err := gitutil.CreateDraftCommitFromTree(treeSHA, sha, changeID)
	if err != nil {
		return err
	}
	if err := gitutil.UpdateRef(syncRef, draftSHA); err != nil {
		return err
	}
	if err := writeWorkspaceLease(repoRoot, workspace, sha); err != nil {
		return err
	}
	if err := gitutil.EnsureHeadRef(repoRoot, workspaceHeadRef(workspace), sha); err != nil {
		return err
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
	if err := gitutil.UpdateRef(workspaceRef, baseSHA); err != nil {
		return "", err
	}
	if err := writeWorkspaceLease(repoRoot, workspace, baseSHA); err != nil {
		return "", err
	}
	if err := ensureWorkspaceConfig(repoRoot, workspace, baseRef, baseSHA); err != nil {
		return "", err
	}
	if err := gitutil.EnsureHeadRef(repoRoot, workspaceHeadRef(workspace), baseSHA); err != nil {
		return "", err
	}
	return draftSHA, nil
}

func resetToDraft(draftSHA string) error {
	if strings.TrimSpace(draftSHA) == "" {
		return fmt.Errorf("draft sha required")
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	return updateWorktreeLocal(repoRoot, draftSHA)
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
	fmt.Fprintln(os.Stdout, "Usage: jul ws [list|checkout|set|new|stack|restack|switch|rename|delete|current]")
}
