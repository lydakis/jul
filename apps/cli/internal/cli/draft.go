package cli

import (
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

func isDraftMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return strings.HasPrefix(trimmed, "[draft]")
}
