package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lydakis/jul/server/internal/storage"
)

const notesRef = "refs/notes/jul/attestations"

func writeAttestationNote(repoPath, commitSHA string, att storage.Attestation) error {
	payload := map[string]any{
		"attestation_id": att.AttestationID,
		"commit_sha":     att.CommitSHA,
		"change_id":      att.ChangeID,
		"type":           att.Type,
		"status":         att.Status,
		"started_at":     att.StartedAt,
		"finished_at":    att.FinishedAt,
		"created_at":     att.CreatedAt,
	}

	if att.SignalsJSON != "" {
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(att.SignalsJSON), &raw); err == nil {
			payload["signals"] = raw
		} else {
			payload["signals_raw"] = att.SignalsJSON
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "--git-dir", repoPath, "notes", "--ref", notesRef, "add", "-f", "-F", "-", commitSHA)
	cmd.Stdin = bytes.NewReader(body)
	cmd.Env = withGitIdentityEnv(os.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git notes failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func withGitIdentityEnv(env []string) []string {
	filtered := make([]string, 0, len(env)+4)
	for _, entry := range env {
		key := entry
		if idx := strings.Index(entry, "="); idx >= 0 {
			key = entry[:idx]
		}
		switch key {
		case "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL":
			continue
		default:
			filtered = append(filtered, entry)
		}
	}
	filtered = append(filtered,
		"GIT_AUTHOR_NAME=Jul Server",
		"GIT_AUTHOR_EMAIL=jul-server@local",
		"GIT_COMMITTER_NAME=Jul Server",
		"GIT_COMMITTER_EMAIL=jul-server@local",
	)
	return filtered
}
