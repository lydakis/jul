package metadata

import (
	"fmt"
	"time"

	"github.com/lydakis/jul/cli/internal/notes"
)

type ChangeCheckpoint struct {
	SHA     string `json:"sha"`
	Message string `json:"message,omitempty"`
}

type PromoteEvent struct {
	Target         string    `json:"target"`
	Strategy       string    `json:"strategy"`
	Timestamp      time.Time `json:"timestamp"`
	Published      []string  `json:"published,omitempty"`
	CheckpointSHAs []string  `json:"checkpoint_shas,omitempty"`
	PublishedSHAs  []string  `json:"published_shas,omitempty"`
	MergeCommitSHA *string   `json:"merge_commit_sha,omitempty"`
	Mainline       *int      `json:"mainline,omitempty"`
}

type ChangeMeta struct {
	ChangeID      string             `json:"change_id"`
	AnchorSHA     string             `json:"anchor_sha"`
	Checkpoints   []ChangeCheckpoint `json:"checkpoints,omitempty"`
	PromoteEvents []PromoteEvent     `json:"promote_events,omitempty"`
}

func ReadChangeMeta(anchorSHA string) (ChangeMeta, bool, error) {
	var meta ChangeMeta
	ok, err := notes.ReadJSON(notes.RefMeta, anchorSHA, &meta)
	if err != nil {
		return ChangeMeta{}, false, err
	}
	return meta, ok, nil
}

func WriteChangeMeta(meta ChangeMeta) error {
	if meta.AnchorSHA == "" {
		return fmt.Errorf("anchor sha required")
	}
	return notes.AddJSON(notes.RefMeta, meta.AnchorSHA, meta)
}
