package metadata

import (
	"encoding/json"
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
		Status:             "pending",
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
	status = normalizeSuggestionStatus(status)
	if limit > 0 {
		return listSuggestionsPaged(changeID, status, limit)
	}
	entries, err := loadSuggestionEntries()
	if err != nil {
		return nil, err
	}
	results := make([]client.Suggestion, 0, len(entries))
	for _, entry := range entries {
		sug := entry.Suggestion
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

func PendingSuggestionCounts() (map[string]int, error) {
	entries, err := loadSuggestionEntries()
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, entry := range entries {
		sug := entry.Suggestion
		if sug.Status != "pending" {
			continue
		}
		changeID := strings.TrimSpace(sug.ChangeID)
		if changeID == "" {
			continue
		}
		counts[changeID]++
	}
	return counts, nil
}

func UpdateSuggestionStatus(id, status, resolution string) (client.Suggestion, error) {
	if strings.TrimSpace(id) == "" {
		return client.Suggestion{}, errors.New("suggestion id required")
	}
	status = normalizeSuggestionStatus(status)
	if status == "" {
		return client.Suggestion{}, errors.New("status required")
	}
	resolution = strings.TrimSpace(resolution)
	entries, err := loadSuggestionEntries()
	if err != nil {
		return client.Suggestion{}, err
	}
	for _, entry := range entries {
		sug := entry.Suggestion
		if sug.SuggestionID != id {
			continue
		}
		sug.Status = status
		if resolution != "" {
			sug.ResolutionMessage = resolution
		}
		if status == "pending" {
			sug.ResolvedAt = time.Time{}
		} else {
			sug.ResolvedAt = time.Now().UTC()
		}
		if err := notes.AddJSON(notes.RefSuggestions, entry.ObjectSHA, sug); err != nil {
			return client.Suggestion{}, err
		}
		return sug, nil
	}
	return client.Suggestion{}, errors.New("suggestion not found")
}

func GetSuggestionByID(id string) (client.Suggestion, bool, error) {
	if strings.TrimSpace(id) == "" {
		return client.Suggestion{}, false, errors.New("suggestion id required")
	}
	entries, err := loadSuggestionEntries()
	if err != nil {
		return client.Suggestion{}, false, err
	}
	for _, entry := range entries {
		sug := entry.Suggestion
		if sug.SuggestionID != id {
			continue
		}
		return sug, true, nil
	}
	return client.Suggestion{}, false, nil
}

func normalizeSuggestionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "open":
		return "pending"
	case "accepted":
		return "applied"
	default:
		return strings.TrimSpace(status)
	}
}

type suggestionEntry struct {
	ObjectSHA  string
	Suggestion client.Suggestion
}

func loadSuggestionEntries() ([]suggestionEntry, error) {
	noteEntries, err := notes.ReadJSONEntries(notes.RefSuggestions)
	if err != nil {
		return nil, err
	}
	results := make([]suggestionEntry, 0, len(noteEntries))
	for _, entry := range noteEntries {
		sug, err := suggestionFromJSONEntry(entry)
		if err != nil {
			return nil, err
		}
		results = append(results, suggestionEntry{
			ObjectSHA:  entry.ObjectSHA,
			Suggestion: sug,
		})
	}
	return results, nil
}

func listSuggestionsPaged(changeID, status string, limit int) ([]client.Suggestion, error) {
	noteEntries, err := notes.List(notes.RefSuggestions)
	if err != nil {
		return nil, err
	}
	if len(noteEntries) == 0 {
		return []client.Suggestion{}, nil
	}

	chunkSize := limit
	if chunkSize < 16 {
		chunkSize = 16
	}
	results := make([]client.Suggestion, 0, limit)

	for end := len(noteEntries); end > 0; {
		start := end - chunkSize
		if start < 0 {
			start = 0
		}
		chunk := noteEntries[start:end]
		jsonChunk, err := notes.ReadJSONEntriesFor(notes.RefSuggestions, chunk)
		if err != nil {
			return nil, err
		}
		for i := len(jsonChunk) - 1; i >= 0; i-- {
			sug, err := suggestionFromJSONEntry(jsonChunk[i])
			if err != nil {
				return nil, err
			}
			if changeID != "" && sug.ChangeID != changeID {
				continue
			}
			if status != "" && sug.Status != status {
				continue
			}
			results = append(results, sug)
			if len(results) >= limit {
				return results[:limit], nil
			}
		}
		end = start
	}
	return results, nil
}

func suggestionFromJSONEntry(entry notes.JSONEntry) (client.Suggestion, error) {
	var sug client.Suggestion
	if err := json.Unmarshal(entry.Payload, &sug); err != nil {
		return client.Suggestion{}, err
	}
	sug.Status = normalizeSuggestionStatus(sug.Status)
	if sug.SuggestedCommitSHA == "" {
		sug.SuggestedCommitSHA = entry.ObjectSHA
	}
	return sug, nil
}
