package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/identity"
	"github.com/lydakis/jul/cli/internal/hooks"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
	wsconfig "github.com/lydakis/jul/cli/internal/workspace"
)

func newInitCommand() Command {
	return Command{
		Name:    "init",
		Summary: "Initialize a Jul-enabled repository",
		Run: func(args []string) int {
			return runInit(args)
		},
	}
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	server := fs.String("server", "", "Git remote base URL (optional)")
	workspace := fs.String("workspace", "", "Workspace name (e.g. @)")
	remote := fs.String("remote", "", "Remote name to configure")
	createRemote := fs.Bool("create-remote", false, "Create or update remote using --server (no API calls)")
	noHooks := fs.Bool("no-hooks", false, "Skip hook installation")
	args = normalizeInitArgs(args)
	_ = fs.Parse(args)

	repoName := strings.TrimSpace(fs.Arg(0))
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		if err := runGit(".", "init"); err != nil {
			fmt.Fprintf(os.Stderr, "git init failed: %v\n", err)
			return 1
		}
		repoRoot, err = gitutil.RepoTopLevel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to locate repo after init: %v\n", err)
			return 1
		}
	}

	if repoName == "" {
		repoName = filepath.Base(repoRoot)
	}
	if repoName == "" {
		fmt.Fprintln(os.Stderr, "repo name required")
		return 1
	}

	if err := ensureJulDir(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize .jul directory: %v\n", err)
		return 1
	}
	deviceID, err := config.DeviceID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create device id: %v\n", err)
		return 1
	}

	if wsName := strings.TrimSpace(*workspace); wsName != "" {
		if err := config.SetRepoConfigValue("workspace", "name", wsName); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set workspace name: %v\n", err)
			return 1
		}
	}
	if err := runGit(repoRoot, "config", "jul.reponame", repoName); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set jul.reponame: %v\n", err)
		return 1
	}

	remoteName := strings.TrimSpace(*remote)
	if remoteName != "" {
		if err := config.SetRepoConfigValue("remote", "name", remoteName); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set remote name: %v\n", err)
			return 1
		}
	}

	if strings.TrimSpace(*server) != "" && *createRemote {
		remoteName = strings.TrimSpace(*remote)
		if remoteName == "" {
			remoteName = "origin"
		}
		remoteURL := buildRemoteURL(*server, repoName)
		if err := configureRemoteURL(repoRoot, remoteName, remoteURL); err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure remote: %v\n", err)
			return 1
		}
	}

	localOnly := false
	remoteName = ""
	selected, err := remotesel.Resolve()
	if err == nil {
		remoteName = selected.Name
		if err := ensureJulRefspecs(repoRoot, selected.Name); err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure remote refspecs: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "Using remote '%s' (%s)\n", selected.Name, selected.URL)
	} else if err == remotesel.ErrMultipleRemote {
		remotes, _ := gitutil.ListRemotes()
		names := []string{}
		for _, rem := range remotes {
			names = append(names, rem.Name)
		}
		if len(names) > 0 {
			fmt.Fprintf(os.Stdout, "Multiple remotes found: %s\n", strings.Join(names, ", "))
		} else {
			fmt.Fprintln(os.Stdout, "Multiple remotes found.")
		}
		fmt.Fprintln(os.Stdout, "Run 'jul remote set <name>' to choose one.")
		localOnly = true
	} else if err == remotesel.ErrNoRemote || err == remotesel.ErrRemoteMissing {
		fmt.Fprintln(os.Stdout, "No remote configured. Working locally.")
		localOnly = true
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve remote: %v\n", err)
		return 1
	}

	if _, err := identity.ResolveUserNamespace(remoteName); err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve user namespace: %v\n", err)
		return 1
	}

	if !*noHooks {
		if _, err := hooks.InstallPostCommit(repoRoot, "jul"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to install hook: %v\n", err)
			return 1
		}
	}

	if _, err := ensureWorkspaceReady(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize workspace: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Device ID: %s\n", deviceID)
	_, workspaceName := workspaceParts()
	if workspaceName == "" {
		workspaceName = "@"
	}
	if localOnly {
		fmt.Fprintf(os.Stdout, "Workspace '%s' ready (local only)\n", workspaceName)
		return 0
	}
	fmt.Fprintf(os.Stdout, "Workspace '%s' ready\n", workspaceName)
	return 0
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return nil
}

func buildRemoteURL(baseURL, repoName string) string {
	base := strings.TrimRight(baseURL, "/")
	name := strings.TrimSpace(repoName)
	if name == "" {
		return base
	}
	if strings.HasSuffix(name, ".git") {
		return base + "/" + name
	}
	return base + "/" + name + ".git"
}

func configureRemoteURL(repoRoot, remoteName, remoteURL string) error {
	if strings.TrimSpace(remoteName) == "" || strings.TrimSpace(remoteURL) == "" {
		return fmt.Errorf("remote name and url required")
	}
	if gitutil.RemoteExists(remoteName) {
		return runGit(repoRoot, "remote", "set-url", remoteName, remoteURL)
	}
	return runGit(repoRoot, "remote", "add", remoteName, remoteURL)
}

func ensureJulDir(repoRoot string) error {
	return os.MkdirAll(filepath.Join(repoRoot, ".jul"), 0o755)
}

func ensureWorkspaceReady(repoRoot string) (string, error) {
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	workspaceRef := workspaceRef(user, workspace)
	syncRef, err := syncRef(user, workspace)
	if err != nil {
		return "", err
	}

	baseSHA := ""
	if gitutil.RefExists(workspaceRef) {
		if sha, err := gitutil.ResolveRef(workspaceRef); err == nil {
			baseSHA = strings.TrimSpace(sha)
		}
	}
	if baseSHA == "" {
		if head, err := gitutil.Git("rev-parse", "HEAD"); err == nil {
			baseSHA = strings.TrimSpace(head)
		}
	}

	draftSHA := ""
	if gitutil.RefExists(syncRef) {
		if sha, err := gitutil.ResolveRef(syncRef); err == nil {
			draftSHA = strings.TrimSpace(sha)
		}
	}

	if draftSHA == "" && baseSHA != "" {
		treeSHA, err := gitutil.DraftTree()
		if err != nil {
			return "", err
		}
		changeID, err := gitutil.NewChangeID()
		if err != nil {
			return "", err
		}
		draftSHA, err = gitutil.CreateDraftCommitFromTree(treeSHA, baseSHA, changeID)
		if err != nil {
			return "", err
		}
	}

	if baseSHA != "" && !gitutil.RefExists(workspaceRef) {
		if err := gitutil.UpdateRef(workspaceRef, baseSHA); err != nil {
			return "", err
		}
	}

	if draftSHA != "" && !gitutil.RefExists(syncRef) {
		if err := gitutil.UpdateRef(syncRef, draftSHA); err != nil {
			return "", err
		}
	}
	if baseSHA == "" && draftSHA != "" {
		if parent, err := gitutil.ParentOf(draftSHA); err == nil {
			baseSHA = strings.TrimSpace(parent)
		}
	}
	if baseSHA != "" {
		if err := writeWorkspaceLease(repoRoot, workspace, baseSHA); err != nil {
			return "", err
		}
	}
	if baseSHA != "" {
		if err := gitutil.EnsureHeadRef(repoRoot, workspaceHeadRef(workspace), baseSHA); err != nil {
			return "", err
		}
	}
	if cfg, ok, err := wsconfig.ReadConfig(repoRoot, workspace); err != nil {
		return "", err
	} else if !ok || strings.TrimSpace(cfg.BaseRef) == "" || strings.TrimSpace(cfg.BaseSHA) == "" {
		baseRef := detectBaseRef(repoRoot)
		if baseSHA == "" {
			if head, err := gitutil.Git("-C", repoRoot, "rev-parse", "HEAD"); err == nil {
				baseSHA = strings.TrimSpace(head)
			}
		}
		if err := ensureWorkspaceConfig(repoRoot, workspace, baseRef, baseSHA); err != nil {
			return "", err
		}
	}
	return draftSHA, nil
}

func normalizeInitArgs(args []string) []string {
	seenPositional := false
	needsReorder := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			if seenPositional {
				needsReorder = true
				break
			}
			continue
		}
		seenPositional = true
	}
	if !needsReorder {
		return args
	}

	flags := []string{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := initFlagName(arg)
			if initFlagTakesValue(name) && !strings.Contains(arg, "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func initFlagName(arg string) string {
	name := strings.TrimLeft(arg, "-")
	if idx := strings.Index(name, "="); idx >= 0 {
		name = name[:idx]
	}
	return name
}

func initFlagTakesValue(name string) bool {
	switch name {
	case "server", "workspace", "remote":
		return true
	default:
		return false
	}
}
