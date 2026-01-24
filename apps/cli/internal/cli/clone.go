package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/hooks"
)

func newCloneCommand() Command {
	return Command{
		Name:    "clone",
		Summary: "Clone a Jul repository and configure remotes",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("clone", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			server := fs.String("server", "", "Jul server base URL")
			remote := fs.String("remote", "jul", "Remote name to configure")
			workspace := fs.String("workspace", "", "Workspace id (user/name)")
			noHooks := fs.Bool("no-hooks", false, "Skip hook installation")
			_ = fs.Parse(args)

			repoArg := strings.TrimSpace(fs.Arg(0))
			if repoArg == "" {
				fmt.Fprintln(os.Stderr, "repo name or URL required")
				return 1
			}
			targetDir := strings.TrimSpace(fs.Arg(1))

			baseURL := strings.TrimRight(strings.TrimSpace(*server), "/")

			repoURL := repoArg
			if !looksLikeURL(repoArg) {
				if baseURL == "" {
					fmt.Fprintln(os.Stderr, "missing --server for non-URL repo name")
					return 1
				}
				repoURL = buildRemoteURL(baseURL, repoArg)
			} else if baseURL == "" {
				baseURL = baseURLFromRepoURL(repoArg)
			}

			cloneArgs := []string{"clone"}
			if strings.TrimSpace(*remote) != "" {
				cloneArgs = append(cloneArgs, "-o", strings.TrimSpace(*remote))
			}
			cloneArgs = append(cloneArgs, repoURL)
			if targetDir != "" {
				cloneArgs = append(cloneArgs, targetDir)
			}
			if err := runGit(".", cloneArgs...); err != nil {
				fmt.Fprintf(os.Stderr, "git clone failed: %v\n", err)
				return 1
			}

			repoRoot, err := resolveCloneRoot(repoURL, targetDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to locate repo root: %v\n", err)
				return 1
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

			repoName := repoNameFromURL(repoURL)
			if repoName != "" {
				if err := runGit(repoRoot, "config", "jul.reponame", repoName); err != nil {
					fmt.Fprintf(os.Stderr, "failed to set jul.reponame: %v\n", err)
					return 1
				}
			}

			if strings.TrimSpace(*remote) != "" {
				remoteName := strings.TrimSpace(*remote)
				_ = runGit(repoRoot, "config", "--unset-all", fmt.Sprintf("remote.%s.fetch", remoteName))
				refspecs := []string{
					"+refs/heads/*:refs/remotes/" + remoteName + "/*",
					"+refs/jul/workspaces/*:refs/jul/workspaces/*",
					"+refs/jul/traces/*:refs/jul/traces/*",
					"+refs/jul/trace-sync/*:refs/jul/trace-sync/*",
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

			fmt.Fprintf(os.Stdout, "Cloned %s\n", repoURL)
			return 0
		},
	}
}

func looksLikeURL(value string) bool {
	return strings.Contains(value, "://") || strings.HasPrefix(value, "git@") || strings.HasPrefix(value, "ssh://")
}

func baseURLFromRepoURL(repoURL string) string {
	if strings.Contains(repoURL, "://") {
		parts := strings.Split(repoURL, "://")
		if len(parts) < 2 {
			return ""
		}
		rest := parts[1]
		host := rest
		if idx := strings.Index(host, "/"); idx >= 0 {
			host = host[:idx]
		}
		return parts[0] + "://" + host
	}
	return ""
}

func repoNameFromURL(repoURL string) string {
	name := repoURL
	if strings.Contains(name, "://") {
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
	} else if strings.Contains(name, ":") && strings.HasPrefix(name, "git@") {
		parts := strings.Split(name, ":")
		name = parts[len(parts)-1]
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
	}
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".git")
	return name
}

func resolveCloneRoot(repoURL, targetDir string) (string, error) {
	if targetDir != "" {
		if filepath.IsAbs(targetDir) {
			return targetDir, nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, targetDir), nil
	}
	repoName := repoNameFromURL(repoURL)
	if repoName == "" {
		return "", fmt.Errorf("unable to determine repo name")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, repoName), nil
}
