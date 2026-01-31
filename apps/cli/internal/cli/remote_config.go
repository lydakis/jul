package cli

import (
	"fmt"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

func ensureJulRefspecs(repoRoot, remoteName string) error {
	if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(remoteName) == "" {
		return fmt.Errorf("repo root and remote name required")
	}
	existing, _ := gitutil.Git("-C", repoRoot, "config", "--get-all", fmt.Sprintf("remote.%s.fetch", remoteName))
	refspecs := map[string]bool{}
	for _, line := range strings.Split(existing, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			refspecs[trimmed] = true
		}
	}
	required := []string{
		"+refs/heads/*:refs/remotes/" + remoteName + "/*",
		"+refs/jul/workspaces/*:refs/jul/workspaces/*",
		"+refs/jul/sync/*:refs/jul/sync/*",
		"+refs/jul/changes/*:refs/jul/changes/*",
		"+refs/jul/anchors/*:refs/jul/anchors/*",
		"+refs/jul/traces/*:refs/jul/traces/*",
		"+refs/jul/trace-sync/*:refs/jul/trace-sync/*",
		"+refs/jul/suggest/*:refs/jul/suggest/*",
		"+refs/jul/keep/*:refs/jul/keep/*",
		"+refs/notes/jul/*:refs/notes/jul/*",
	}
	for _, refspec := range required {
		if refspecs[refspec] {
			continue
		}
		if _, err := gitutil.Git("-C", repoRoot, "config", "--add", fmt.Sprintf("remote.%s.fetch", remoteName), refspec); err != nil {
			return err
		}
	}
	return nil
}
