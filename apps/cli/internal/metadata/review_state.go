package metadata

import (
	"fmt"
	"time"

	"github.com/lydakis/jul/cli/internal/notes"
)

type ReviewState struct {
	ChangeID        string    `json:"change_id"`
	AnchorSHA       string    `json:"anchor_sha"`
	LatestCheckpoint string    `json:"latest_checkpoint"`
	Status          string    `json:"status"`
	WorkspaceID     string    `json:"workspace_id,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func ReadReviewState(anchorSHA string) (ReviewState, bool, error) {
	var state ReviewState
	ok, err := notes.ReadJSON(notes.RefReviewState, anchorSHA, &state)
	if err != nil {
		return ReviewState{}, false, err
	}
	return state, ok, nil
}

func WriteReviewState(state ReviewState) error {
	if state.AnchorSHA == "" {
		return fmt.Errorf("anchor sha required")
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if state.Status == "" {
		state.Status = "open"
	}
	return notes.AddJSON(notes.RefReviewState, state.AnchorSHA, state)
}
