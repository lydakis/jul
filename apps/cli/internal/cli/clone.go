package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/hooks"
	"github.com/lydakis/jul/cli/internal/output"
)

type cloneOutput struct {
	Status         string `json:"status"`
	RepoURL        string `json:"repo_url"`
	RepoRoot       string `json:"repo_root"`
	RemoteName     string `json:"remote_name,omitempty"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	HooksInstalled bool   `json:"hooks_installed,omitempty"`
}

func newCloneCommand() Command {
	return Command{
		Name:    "clone",
		Summary: "Clone a Jul repository and configure remotes",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("clone")
			server := fs.String("server", "", "Jul server base URL")
			remote := fs.String("remote", "jul", "Remote name to configure")
			workspace := fs.String("workspace", "", "Workspace id (user/name)")
			noHooks := fs.Bool("no-hooks", false, "Skip hook installation")
			_ = fs.Parse(args)

			repoArg := strings.TrimSpace(fs.Arg(0))
			if repoArg == "" {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "clone_missing_repo", "repo name or URL required", nil)
				} else {
					fmt.Fprintln(os.Stderr, "repo name or URL required")
				}
				return 1
			}
			targetDir := strings.TrimSpace(fs.Arg(1))

			baseURL := strings.TrimRight(strings.TrimSpace(*server), "/")

			repoURL := repoArg
			if !looksLikeURL(repoArg) {
				if baseURL == "" {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "clone_missing_server", "missing --server for non-URL repo name", nil)
					} else {
						fmt.Fprintln(os.Stderr, "missing --server for non-URL repo name")
					}
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
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "clone_failed", fmt.Sprintf("git clone failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "git clone failed: %v\n", err)
				}
				return 1
			}

			repoRoot, err := resolveCloneRoot(repoURL, targetDir)
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "clone_resolve_root_failed", fmt.Sprintf("failed to locate repo root: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to locate repo root: %v\n", err)
				}
				return 1
			}

			wsID := strings.TrimSpace(*workspace)
			if wsID == "" {
				wsID = config.WorkspaceID()
			}
			if wsID != "" {
				if err := runGit(repoRoot, "config", "jul.workspace", wsID); err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "clone_set_workspace_failed", fmt.Sprintf("failed to set jul.workspace: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to set jul.workspace: %v\n", err)
					}
					return 1
				}
			}

			repoName := repoNameFromURL(repoURL)
			if repoName != "" {
				if err := runGit(repoRoot, "config", "jul.reponame", repoName); err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "clone_set_reponame_failed", fmt.Sprintf("failed to set jul.reponame: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to set jul.reponame: %v\n", err)
					}
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
						if *jsonOut {
							_ = output.EncodeError(os.Stdout, "clone_set_refspec_failed", fmt.Sprintf("failed to add refspec: %v", err), nil)
						} else {
							fmt.Fprintf(os.Stderr, "failed to add refspec: %v\n", err)
						}
						return 1
					}
				}
			}

			if !*noHooks {
				if _, err := hooks.InstallPostCommit(repoRoot, "jul"); err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "clone_install_hook_failed", fmt.Sprintf("failed to install hook: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to install hook: %v\n", err)
					}
					return 1
				}
			}

			out := cloneOutput{
				Status:         "ok",
				RepoURL:        repoURL,
				RepoRoot:       repoRoot,
				RemoteName:     strings.TrimSpace(*remote),
				WorkspaceID:    wsID,
				HooksInstalled: !*noHooks,
			}
			if *jsonOut {
				return writeJSON(out)
			}
			renderCloneOutput(out)
			return 0
		},
	}
}

func renderCloneOutput(out cloneOutput) {
	fmt.Fprintf(os.Stdout, "Cloned %s\n", out.RepoURL)
	if out.RepoRoot != "" {
		fmt.Fprintf(os.Stdout, "Repo root: %s\n", out.RepoRoot)
	}
	if out.RemoteName != "" {
		fmt.Fprintf(os.Stdout, "Remote: %s\n", out.RemoteName)
	}
	if out.WorkspaceID != "" {
		fmt.Fprintf(os.Stdout, "Workspace: %s\n", out.WorkspaceID)
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
