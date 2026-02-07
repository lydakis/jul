package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

const checkpointMessageFilesLimit = 200

func resolveCheckpointMessage(message string, stream io.Writer) (string, error) {
	if strings.TrimSpace(message) != "" {
		return message, nil
	}
	generated, err := generateCheckpointMessage(stream)
	if err != nil {
		return "", err
	}
	generated = sanitizeGeneratedCheckpointMessage(generated)
	if generated == "" {
		return "", fmt.Errorf("agent returned empty checkpoint message")
	}
	return generated, nil
}

func sanitizeGeneratedCheckpointMessage(message string) string {
	lines := strings.Split(strings.TrimSpace(message), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if isReservedCheckpointTrailer(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func isReservedCheckpointTrailer(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "trace-head:") || strings.HasPrefix(lower, "trace-base:") {
		return true
	}
	if strings.HasPrefix(lower, "change-id:") {
		return true
	}
	if strings.HasPrefix(lower, "change-id ") {
		value := strings.TrimSpace(trimmed[len("change-id "):])
		return strings.HasPrefix(value, "I") && len(value) == 41
	}
	return false
}

func generateCheckpointMessage(stream io.Writer) (string, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return "", err
	}
	draftSHA, parentSHA, err := currentDraftAndBase()
	if err != nil {
		return "", err
	}

	changeID := ""
	if strings.TrimSpace(draftSHA) != "" {
		msg, _ := gitutil.CommitMessage(draftSHA)
		changeID = gitutil.ExtractChangeID(msg)
		if changeID == "" {
			changeID = gitutil.FallbackChangeID(draftSHA)
		}
	}

	treeSHA, err := gitutil.DraftTree()
	if err != nil {
		return "", err
	}

	req := agent.ReviewRequest{
		Version:       1,
		Action:        "generate_message",
		WorkspacePath: repoRoot,
		Context: agent.ReviewContext{
			Checkpoint: strings.TrimSpace(parentSHA),
			ChangeID:   strings.TrimSpace(changeID),
			Diff:       checkpointMessageContext(parentSHA, treeSHA),
		},
	}

	provider, err := agent.ResolveProvider()
	if err != nil {
		return "", err
	}
	resp, err := agent.RunReviewWithStream(context.Background(), provider, req, stream)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Summary), nil
}

func checkpointMessageContext(parentSHA, treeSHA string) string {
	parentSHA = strings.TrimSpace(parentSHA)
	treeSHA = strings.TrimSpace(treeSHA)
	if parentSHA == "" || treeSHA == "" {
		return ""
	}

	var context strings.Builder
	if shortStat, err := gitutil.Git("diff", "--shortstat", parentSHA, treeSHA); err == nil {
		shortStat = strings.TrimSpace(shortStat)
		if shortStat != "" {
			context.WriteString("Summary:\n")
			context.WriteString(shortStat)
			context.WriteString("\n")
		}
	}

	if fileList, err := gitutil.Git("diff", "--name-status", parentSHA, treeSHA); err == nil {
		fileList = strings.TrimSpace(fileList)
		if fileList != "" {
			if context.Len() > 0 {
				context.WriteString("\n")
			}
			context.WriteString("Files:\n")
			lines := strings.Split(fileList, "\n")
			if len(lines) > checkpointMessageFilesLimit {
				lines = append(lines[:checkpointMessageFilesLimit], fmt.Sprintf("... (%d more)", len(lines)-checkpointMessageFilesLimit))
			}
			context.WriteString(strings.Join(lines, "\n"))
			context.WriteString("\n")
		}
	}

	return strings.TrimSpace(context.String())
}
