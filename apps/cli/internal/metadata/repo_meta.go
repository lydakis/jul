package metadata

import (
	"time"

	"github.com/lydakis/jul/cli/internal/notes"
)

type RepoMeta struct {
	RepoID        string `json:"repo_id"`
	UserNamespace string `json:"user_namespace"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

func ReadRepoMeta(rootSHA string) (RepoMeta, bool, error) {
	var meta RepoMeta
	ok, err := notes.ReadJSON(notes.RefRepoMeta, rootSHA, &meta)
	return meta, ok, err
}

func WriteRepoMeta(rootSHA string, meta RepoMeta) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if meta.CreatedAt == "" {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now
	return notes.AddJSON(notes.RefRepoMeta, rootSHA, meta)
}
