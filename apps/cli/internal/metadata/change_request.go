package metadata

import (
	"fmt"
	"time"

	"github.com/lydakis/jul/cli/internal/notes"
)

type ChangeRequestState struct {
	ChangeID         string    `json:"change_id"`
	AnchorSHA        string    `json:"anchor_sha"`
	LatestCheckpoint string    `json:"latest_checkpoint"`
	Status           string    `json:"status"`
	WorkspaceID      string    `json:"workspace_id,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func ReadChangeRequestState(anchorSHA string) (ChangeRequestState, bool, error) {
	var state ChangeRequestState
	ok, err := notes.ReadJSON(notes.RefCRState, anchorSHA, &state)
	if err != nil {
		return ChangeRequestState{}, false, err
	}
	return state, ok, nil
}

func WriteChangeRequestState(state ChangeRequestState) error {
	if state.AnchorSHA == "" {
		return fmt.Errorf("anchor sha required")
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if state.Status == "" {
		state.Status = "open"
	}
	return notes.AddJSON(notes.RefCRState, state.AnchorSHA, state)
}
