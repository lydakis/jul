package cli

import (
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/workspace"
)

func detectBaseRef(repoRoot string) string {
	if strings.TrimSpace(repoRoot) == "" {
		return ""
	}
	if ref, err := gitutil.Git("-C", repoRoot, "symbolic-ref", "-q", "HEAD"); err == nil {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			return ref
		}
	}
	if refExists(repoRoot, "refs/heads/main") {
		return "refs/heads/main"
	}
	if refExists(repoRoot, "refs/heads/master") {
		return "refs/heads/master"
	}
	return ""
}

func refExists(repoRoot, ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return false
	}
	_, err := gitutil.Git("-C", repoRoot, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func ensureWorkspaceConfig(repoRoot, workspaceName, baseRef, baseSHA string) error {
	cfg := workspace.Config{
		BaseRef: strings.TrimSpace(baseRef),
		BaseSHA: strings.TrimSpace(baseSHA),
	}
	return workspace.WriteConfig(repoRoot, workspaceName, cfg)
}
