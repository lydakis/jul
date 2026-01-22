package metadata

import (
	"fmt"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/notes"
)

type PromptNote struct {
	CommitSHA string    `json:"commit_sha"`
	ChangeID  string    `json:"change_id,omitempty"`
	Source    string    `json:"source,omitempty"`
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"created_at"`
}

func WritePrompt(note PromptNote) error {
	if strings.TrimSpace(note.CommitSHA) == "" {
		return fmt.Errorf("commit sha required")
	}
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now().UTC()
	}
	return notes.AddJSON(notes.RefPrompts, note.CommitSHA, note)
}

func GetPrompt(commitSHA string) (*PromptNote, error) {
	var note PromptNote
	found, err := notes.ReadJSON(notes.RefPrompts, commitSHA, &note)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &note, nil
}
