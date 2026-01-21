package metadata

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/notes"
)

type SuggestionCreate struct {
	ChangeID           string
	BaseCommitSHA      string
	SuggestedCommitSHA string
	CreatedBy          string
	Reason             string
	Description        string
	Confidence         float64
}

func CreateSuggestion(req SuggestionCreate) (client.Suggestion, error) {
	if strings.TrimSpace(req.BaseCommitSHA) == "" || strings.TrimSpace(req.SuggestedCommitSHA) == "" {
		return client.Suggestion{}, errors.New("base and suggested commit required")
	}
	changeID := strings.TrimSpace(req.ChangeID)
	if changeID == "" {
		if msg, err := gitutil.CommitMessage(req.BaseCommitSHA); err == nil {
			changeID = gitutil.ExtractChangeID(msg)
		}
		if changeID == "" {
			changeID = gitutil.FallbackChangeID(req.BaseCommitSHA)
		}
	}
	if changeID == "" {
		return client.Suggestion{}, errors.New("change id missing")
	}

	suggestion := client.Suggestion{
		SuggestionID:       newID(),
		ChangeID:           changeID,
		BaseCommitSHA:      req.BaseCommitSHA,
		SuggestedCommitSHA: req.SuggestedCommitSHA,
		CreatedBy:          strings.TrimSpace(req.CreatedBy),
		Reason:             strings.TrimSpace(req.Reason),
		Description:        strings.TrimSpace(req.Description),
		Confidence:         req.Confidence,
		Status:             "open",
		CreatedAt:          time.Now().UTC(),
	}
	if suggestion.CreatedBy == "" {
		suggestion.CreatedBy = "user"
	}
	if suggestion.Reason == "" {
		suggestion.Reason = "unspecified"
	}

	ref := fmt.Sprintf("refs/jul/suggest/%s/%s", changeID, suggestion.SuggestionID)
	if err := gitutil.UpdateRef(ref, suggestion.SuggestedCommitSHA); err != nil {
		return client.Suggestion{}, err
	}
	if err := notes.AddJSON(notes.RefSuggestions, suggestion.SuggestedCommitSHA, suggestion); err != nil {
		return client.Suggestion{}, err
	}
	return suggestion, nil
}

func ListSuggestions(changeID, status string, limit int) ([]client.Suggestion, error) {
	entries, err := notes.List(notes.RefSuggestions)
	if err != nil {
		return nil, err
	}
	results := make([]client.Suggestion, 0, len(entries))
	for _, entry := range entries {
		var sug client.Suggestion
		found, err := notes.ReadJSON(notes.RefSuggestions, entry.ObjectSHA, &sug)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if sug.SuggestedCommitSHA == "" {
			sug.SuggestedCommitSHA = entry.ObjectSHA
		}
		if changeID != "" && sug.ChangeID != changeID {
			continue
		}
		if status != "" && sug.Status != status {
			continue
		}
		results = append(results, sug)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if limit <= 0 || len(results) <= limit {
		return results, nil
	}
	return results[:limit], nil
}

func UpdateSuggestionStatus(id, status string) (client.Suggestion, error) {
	if strings.TrimSpace(id) == "" {
		return client.Suggestion{}, errors.New("suggestion id required")
	}
	entries, err := notes.List(notes.RefSuggestions)
	if err != nil {
		return client.Suggestion{}, err
	}
	for _, entry := range entries {
		var sug client.Suggestion
		found, err := notes.ReadJSON(notes.RefSuggestions, entry.ObjectSHA, &sug)
		if err != nil {
			return client.Suggestion{}, err
		}
		if !found {
			continue
		}
		if sug.SuggestionID != id {
			continue
		}
		sug.Status = status
		sug.ResolvedAt = time.Now().UTC()
		if sug.SuggestedCommitSHA == "" {
			sug.SuggestedCommitSHA = entry.ObjectSHA
		}
		if err := notes.AddJSON(notes.RefSuggestions, entry.ObjectSHA, sug); err != nil {
			return client.Suggestion{}, err
		}
		return sug, nil
	}
	return client.Suggestion{}, errors.New("suggestion not found")
}
