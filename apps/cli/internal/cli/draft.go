package cli

import (
	"fmt"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
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

func isDraftMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return strings.HasPrefix(trimmed, "[draft]")
}
