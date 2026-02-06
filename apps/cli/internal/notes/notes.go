package notes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

const (
	RefAttestationsTrace      = "refs/notes/jul/attestations/trace"
	RefAttestationsCheckpoint = "refs/notes/jul/attestations/checkpoint"
	RefTraces                 = "refs/notes/jul/traces"
	RefSuggestions            = "refs/notes/jul/suggestions"
	RefAgentReview            = "refs/notes/jul/agent-review"
	RefCRState                = "refs/notes/jul/cr-state"
	RefCRComments             = "refs/notes/jul/cr-comments"
	RefMeta                   = "refs/notes/jul/meta"
	RefRepoMeta               = "refs/notes/jul/repo-meta"
	RefChangeID               = "refs/notes/jul/change-id"
)

const MaxNoteSize = 16 * 1024

var ErrNoteTooLarge = errors.New("note exceeds max size")
var ErrRepoRequired = errors.New("jul repository required")

var (
	notesRefCacheMu sync.Mutex
	notesRefCache   = map[string]bool{}
)

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
	repoRoot, err := notesRepoRoot()
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", repoRoot, "notes", "--ref", ref, "add", "-f", "-F", "-", objectSHA)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if isRepoFailure(output) {
			return ErrRepoRequired
		}
		return fmt.Errorf("jul failed to write note")
	}
	cacheNotesRefExists(repoRoot, ref, true)
	return nil
}

func ReadJSON(ref, objectSHA string, target any) (bool, error) {
	if strings.TrimSpace(ref) == "" || strings.TrimSpace(objectSHA) == "" {
		return false, fmt.Errorf("note ref and object sha required")
	}
	repoRoot, err := notesRepoRoot()
	if err != nil {
		return false, err
	}
	exists, err := notesRefExists(repoRoot, ref)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "notes", "--ref", ref, "show", objectSHA)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if isRepoFailure(output) {
			return false, ErrRepoRequired
		}
		if isNoteMissing(output) {
			return false, nil
		}
		return false, fmt.Errorf("jul failed to read note")
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

func Remove(ref, objectSHA string) error {
	if strings.TrimSpace(ref) == "" || strings.TrimSpace(objectSHA) == "" {
		return fmt.Errorf("note ref and object sha required")
	}
	repoRoot, err := notesRepoRoot()
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", repoRoot, "notes", "--ref", ref, "remove", objectSHA)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if isRepoFailure(output) {
			return ErrRepoRequired
		}
		if isNoteMissing(output) {
			return nil
		}
		return fmt.Errorf("jul failed to remove note")
	}
	return nil
}

func List(ref string) ([]Entry, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("note ref required")
	}
	repoRoot, err := notesRepoRoot()
	if err != nil {
		return nil, err
	}
	exists, err := notesRefExists(repoRoot, ref)
	if err != nil {
		return nil, err
	}
	if !exists {
		return []Entry{}, nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "notes", "--ref", ref, "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if isRepoFailure(output) {
			return nil, ErrRepoRequired
		}
		return nil, fmt.Errorf("jul failed to list notes")
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

func notesRepoRoot() (string, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil || strings.TrimSpace(repoRoot) == "" {
		return "", ErrRepoRequired
	}
	return repoRoot, nil
}

func notesRefExists(repoRoot, ref string) (bool, error) {
	if cached, ok := loadCachedNotesRefExists(repoRoot, ref); ok && cached {
		return cached, nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "show-ref", "--verify", "--quiet", ref)
	output, err := cmd.CombinedOutput()
	if err == nil {
		cacheNotesRefExists(repoRoot, ref, true)
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 {
			if isRepoFailure(output) {
				return false, ErrRepoRequired
			}
			// Exit code 1 means the ref does not exist.
			return false, nil
		}
	}
	if isRepoFailure(output) {
		return false, ErrRepoRequired
	}
	return false, fmt.Errorf("jul failed to verify notes ref")
}

func isRepoFailure(output []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(string(output)))
	return strings.Contains(msg, "not a git repository") || strings.Contains(msg, "unable to read current working directory")
}

func loadCachedNotesRefExists(repoRoot, ref string) (bool, bool) {
	key := notesRefCacheKey(repoRoot, ref)
	notesRefCacheMu.Lock()
	defer notesRefCacheMu.Unlock()
	exists, ok := notesRefCache[key]
	return exists, ok
}

func cacheNotesRefExists(repoRoot, ref string, exists bool) {
	key := notesRefCacheKey(repoRoot, ref)
	notesRefCacheMu.Lock()
	defer notesRefCacheMu.Unlock()
	notesRefCache[key] = exists
}

func notesRefCacheKey(repoRoot, ref string) string {
	return strings.TrimSpace(repoRoot) + "\x00" + strings.TrimSpace(ref)
}
