package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
)

func localReflogEntries(limit int) ([]output.ReflogEntry, error) {
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	ref := workspaceRef(user, workspace)
	if !gitutil.RefExists(ref) {
		return []output.ReflogEntry{}, nil
	}

	args := []string{"reflog", "show", "--date=iso-strict", "--format=%H%x1f%gs%x1f%cd", ref}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}
	out, err := gitutil.Git(args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	entries := make([]output.ReflogEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) < 3 {
			continue
		}
		sha := strings.TrimSpace(parts[0])
		reflogMsg := strings.TrimSpace(parts[1])
		when := strings.TrimSpace(parts[2])
		msg := ""
		kind := "checkpoint"
		if commitMsg, err := gitutil.CommitMessage(sha); err == nil {
			msg = firstLine(commitMsg)
			if isDraftMessage(commitMsg) {
				kind = "draft"
			}
		}
		if kind == "draft" && reflogMsg != "" {
			msg = reflogMsg
		}
		when = normalizeWhen(when)
		entries = append(entries, output.ReflogEntry{
			CommitSHA: sha,
			Kind:      kind,
			Message:   msg,
			When:      when,
		})
	}
	return entries, nil
}

func normalizeWhen(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Format(time.RFC3339)
	}
	return value
}
