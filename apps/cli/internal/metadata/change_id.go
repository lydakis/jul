package metadata

import (
	"fmt"
	"strings"

	"github.com/lydakis/jul/cli/internal/notes"
)

// ChangeIDNote records reverse index metadata for published commits.
type ChangeIDNote struct {
	ChangeID            string   `json:"change_id"`
	PromoteEventID      int      `json:"promote_event_id,omitempty"`
	Strategy            string   `json:"strategy,omitempty"`
	SourceCheckpointSHA string   `json:"source_checkpoint_sha,omitempty"`
	CheckpointSHAs      []string `json:"checkpoint_shas,omitempty"`
	TraceBase           string   `json:"trace_base,omitempty"`
	TraceHead           string   `json:"trace_head,omitempty"`
}

func WriteChangeIDNote(publishedSHA string, note ChangeIDNote) error {
	if strings.TrimSpace(publishedSHA) == "" {
		return fmt.Errorf("published sha required")
	}
	if strings.TrimSpace(note.ChangeID) == "" {
		return fmt.Errorf("change id required")
	}
	return notes.AddJSON(notes.RefChangeID, publishedSHA, note)
}
