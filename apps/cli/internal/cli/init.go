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
	"github.com/lydakis/jul/cli/internal/hooks"
)

func newInitCommand() Command {
	return Command{
		Name:    "init",
		Summary: "Initialize a Jul-enabled repository",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("init", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			server := fs.String("server", "", "Jul server base URL")
			workspace := fs.String("workspace", "", "Workspace id (user/name)")
			remote := fs.String("remote", "jul", "Remote name to configure")
			createRemote := fs.Bool("create-remote", false, "Force server repo creation")
			noCreate := fs.Bool("no-create", false, "Skip server repo creation")
			noHooks := fs.Bool("no-hooks", false, "Skip hook installation")
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

			baseURL := strings.TrimSpace(*server)
			if baseURL == "" {
				baseURL = config.BaseURL()
			}
			baseURL = strings.TrimRight(baseURL, "/")

			shouldCreateRemote := config.CreateRemoteDefault()
			if *createRemote {
				shouldCreateRemote = true
			}
			if *noCreate {
				shouldCreateRemote = false
			}

			var cloneURL string
			if baseURL != "" && shouldCreateRemote {
				cli := client.New(baseURL)
				created, err := cli.CreateRepo(repoName)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to create repo on server: %v\n", err)
					return 1
				}
				cloneURL = created.CloneURL
			}

			if baseURL != "" {
				if err := runGit(repoRoot, "config", "jul.baseurl", baseURL); err != nil {
					fmt.Fprintf(os.Stderr, "failed to set jul.baseurl: %v\n", err)
					return 1
				}
			}

			wsID := strings.TrimSpace(*workspace)
			if wsID == "" {
				wsID = config.WorkspaceID()
			}
			if wsID != "" {
				if err := runGit(repoRoot, "config", "jul.workspace", wsID); err != nil {
					fmt.Fprintf(os.Stderr, "failed to set jul.workspace: %v\n", err)
					return 1
				}
			}
			if err := runGit(repoRoot, "config", "jul.reponame", repoName); err != nil {
				fmt.Fprintf(os.Stderr, "failed to set jul.reponame: %v\n", err)
				return 1
			}

			if baseURL != "" && strings.TrimSpace(*remote) != "" {
				remoteName := strings.TrimSpace(*remote)
				remoteURL := cloneURL
				if remoteURL == "" {
					remoteURL = buildRemoteURL(baseURL, repoName)
				}
				if err := runGit(repoRoot, "config", fmt.Sprintf("remote.%s.url", remoteName), remoteURL); err != nil {
					fmt.Fprintf(os.Stderr, "failed to set remote url: %v\n", err)
					return 1
				}
				_ = runGit(repoRoot, "config", "--unset-all", fmt.Sprintf("remote.%s.fetch", remoteName))
				refspecs := []string{
					"+refs/heads/*:refs/remotes/" + remoteName + "/*",
					"+refs/jul/workspaces/*:refs/jul/workspaces/*",
					"+refs/jul/suggest/*:refs/jul/suggest/*",
					"+refs/notes/jul/*:refs/notes/jul/*",
				}
				for _, refspec := range refspecs {
					if err := runGit(repoRoot, "config", "--add", fmt.Sprintf("remote.%s.fetch", remoteName), refspec); err != nil {
						fmt.Fprintf(os.Stderr, "failed to add refspec: %v\n", err)
						return 1
					}
				}
			}

			if !*noHooks {
				if _, err := hooks.InstallPostCommit(repoRoot, "jul"); err != nil {
					fmt.Fprintf(os.Stderr, "failed to install hook: %v\n", err)
					return 1
				}
			}

			fmt.Fprintf(os.Stdout, "Initialized Jul repo %s\n", repoName)
			return 0
		},
	}
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
