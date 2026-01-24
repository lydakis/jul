package metadata

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/lydakis/jul/cli/internal/notes"
)

type AgentReviewNote struct {
	ReviewID      string          `json:"review_id"`
	BaseCommitSHA string          `json:"base_commit_sha"`
	ChangeID      string          `json:"change_id"`
	Status        string          `json:"status"`
	CreatedBy     string          `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
	Response      json.RawMessage `json:"response,omitempty"`
}

func WriteAgentReview(note AgentReviewNote) (AgentReviewNote, error) {
	if note.ReviewID == "" {
		note.ReviewID = newID()
	}
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now().UTC()
	}
	if note.CreatedBy == "" {
		note.CreatedBy = "agent"
	}
	stored := note
	for attempt := 0; attempt < 2; attempt++ {
		if err := notes.AddJSON(notes.RefAgentReview, stored.BaseCommitSHA, stored); err != nil {
			if errors.Is(err, notes.ErrNoteTooLarge) {
				stored.Response = nil
				continue
			}
			return AgentReviewNote{}, err
		}
		return stored, nil
	}
	return AgentReviewNote{}, errors.New("agent review note exceeds size limit")
}
