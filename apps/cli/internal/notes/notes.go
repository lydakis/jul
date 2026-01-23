package notes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

const (
	RefAttestationsTrace      = "refs/notes/jul/attestations/trace"
	RefAttestationsCheckpoint = "refs/notes/jul/attestations/checkpoint"
	RefTraces                 = "refs/notes/jul/traces"
	RefSuggestions            = "refs/notes/jul/suggestions"
	RefReview                 = "refs/notes/jul/review"
	RefMeta                   = "refs/notes/jul/meta"
)

const MaxNoteSize = 16 * 1024

var ErrNoteTooLarge = errors.New("note exceeds max size")

type Entry struct {
	ObjectSHA string
	NoteSHA   string
}

func AddJSON(ref, objectSHA string, payload any) error {
	if strings.TrimSpace(ref) == "" || strings.TrimSpace(objectSHA) == "" {
		return fmt.Errorf("note ref and object sha required")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if len(data) > MaxNoteSize {
		return fmt.Errorf("%w: %d bytes", ErrNoteTooLarge, len(data))
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", repoRoot, "notes", "--ref", ref, "add", "-f", "-F", "-", objectSHA)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git notes add failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func ReadJSON(ref, objectSHA string, target any) (bool, error) {
	if strings.TrimSpace(ref) == "" || strings.TrimSpace(objectSHA) == "" {
		return false, fmt.Errorf("note ref and object sha required")
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return false, err
	}
	cmd := exec.Command("git", "-C", repoRoot, "notes", "--ref", ref, "show", objectSHA)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if isNoteMissing(output) {
			return false, nil
		}
		return false, fmt.Errorf("git notes show failed: %s", strings.TrimSpace(string(output)))
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(trimmed), target); err != nil {
		return false, err
	}
	return true, nil
}

func List(ref string) ([]Entry, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("note ref required")
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "-C", repoRoot, "notes", "--ref", ref, "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git notes list failed: %s", strings.TrimSpace(string(output)))
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return []Entry{}, nil
	}
	entries := make([]Entry, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		entries = append(entries, Entry{
			NoteSHA:   fields[0],
			ObjectSHA: fields[1],
		})
	}
	return entries, nil
}

func isNoteMissing(output []byte) bool {
	msg := strings.ToLower(string(output))
	return strings.Contains(msg, "no note found") || strings.Contains(msg, "no notes")
}
