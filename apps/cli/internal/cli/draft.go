package cli

import (
	"fmt"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/workspace"
)

func currentDraftSHA() (string, error) {
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	if ref, err := syncRef(user, workspace); err == nil {
		if gitutil.RefExists(ref) {
			if sha, err := gitutil.ResolveRef(ref); err == nil {
				return sha, nil
			}
		}
	}
	ref := workspaceRef(user, workspace)
	if gitutil.RefExists(ref) {
		if sha, err := gitutil.ResolveRef(ref); err == nil {
			return sha, nil
		}
	}
	return gitutil.Git("rev-parse", "HEAD")
}

func currentBaseSHA() (string, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err == nil {
		_, wsName := workspaceParts()
		if wsName == "" {
			wsName = "@"
		}
		if cfg, ok, err := workspace.ReadConfig(repoRoot, wsName); err == nil && ok {
			if strings.TrimSpace(cfg.BaseSHA) != "" {
				return strings.TrimSpace(cfg.BaseSHA), nil
			}
		}
	}
	draftSHA, err := currentDraftSHA()
	if err != nil {
		return "", err
	}
	if parent, err := gitutil.ParentOf(draftSHA); err == nil && strings.TrimSpace(parent) != "" {
		return strings.TrimSpace(parent), nil
	}
	if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
		return checkpoint.SHA, nil
	}
	return "", fmt.Errorf("base commit not found")
}

func currentDraftAndBase() (string, string, error) {
	draftSHA, err := currentDraftSHA()
	if err != nil {
		return "", "", err
	}
	baseSHA, err := currentBaseSHA()
	if err != nil {
		parentSHA, _ := gitutil.ParentOf(draftSHA)
		baseSHA = strings.TrimSpace(parentSHA)
	}
	return strings.TrimSpace(draftSHA), strings.TrimSpace(baseSHA), nil
}

func suggestionIsStale(baseSHA, draftSHA, parentSHA string) bool {
	base := strings.TrimSpace(baseSHA)
	if base == "" {
		return false
	}
	draft := strings.TrimSpace(draftSHA)
	parent := strings.TrimSpace(parentSHA)
	if base == draft {
		return false
	}
	if parent != "" && base == parent {
		return false
	}
	return true
}

func isDraftMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return strings.HasPrefix(trimmed, "[draft]")
}
