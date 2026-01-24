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
			case "set", "new":
				return runWorkspaceSet(args[1:])
			case "switch":
				return runWorkspaceSwitch(args[1:])
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

	wsID := name
	if !strings.Contains(name, "/") {
		owner := strings.TrimSpace(*user)
		if owner == "" {
			owner = config.ServerUser()
		}
		if owner == "" {
			owner = strings.Split(config.WorkspaceID(), "/")[0]
		}
		wsID = owner + "/" + name
	}

	if err := runGitConfig("jul.workspace", wsID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Workspace set to %s\n", wsID)
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

	if _, err := gitutil.Git("-C", repoRoot, "reset", "--hard", sha); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update working tree: %v\n", err)
		return 1
	}
	if _, err := gitutil.Git("-C", repoRoot, "clean", "-fd"); err != nil {
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

func runWorkspaceSwitch(args []string) int {
	return runWorkspaceSet(args)
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
	fmt.Fprintln(os.Stdout, "Usage: jul ws [list|checkout|set|new|switch|rename|delete|current]")
}
