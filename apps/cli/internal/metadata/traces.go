package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/notes"
)

type TraceNote struct {
	TraceSHA      string    `json:"trace_sha"`
	TraceType     string    `json:"trace_type,omitempty"`
	PromptHash    string    `json:"prompt_hash,omitempty"`
	PromptSummary string    `json:"prompt_summary,omitempty"`
	PromptFull    string    `json:"prompt_full,omitempty"`
	Agent         string    `json:"agent,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	Turn          int       `json:"turn,omitempty"`
	Device        string    `json:"device,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func WriteTrace(note TraceNote) error {
	if strings.TrimSpace(note.TraceSHA) == "" {
		return fmt.Errorf("trace sha required")
	}
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now().UTC()
	}
	return notes.AddJSON(notes.RefTraces, note.TraceSHA, note)
}

func GetTrace(traceSHA string) (*TraceNote, error) {
	var note TraceNote
	found, err := notes.ReadJSON(notes.RefTraces, traceSHA, &note)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &note, nil
}

func WriteTracePrompt(traceSHA, prompt string) error {
	return writeTraceLocal(traceSHA, "prompts", prompt)
}

func WriteTraceSummary(traceSHA, summary string) error {
	return writeTraceLocal(traceSHA, "summaries", summary)
}

func ReadTracePrompt(traceSHA string) (string, error) {
	return readTraceLocal(traceSHA, "prompts")
}

func ReadTraceSummary(traceSHA string) (string, error) {
	return readTraceLocal(traceSHA, "summaries")
}

func writeTraceLocal(traceSHA, dir, content string) error {
	if strings.TrimSpace(traceSHA) == "" {
		return fmt.Errorf("trace sha required")
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	base := filepath.Join(root, ".jul", "traces", dir)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	path := filepath.Join(base, traceSHA+".txt")
	return os.WriteFile(path, []byte(content+"\n"), 0o644)
}

func readTraceLocal(traceSHA, dir string) (string, error) {
	if strings.TrimSpace(traceSHA) == "" {
		return "", fmt.Errorf("trace sha required")
	}
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, ".jul", "traces", dir, traceSHA+".txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
