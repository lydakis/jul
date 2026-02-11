package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	sorted, err := listSuggestionsSorted()
	if err != nil {
		return nil, err
	}
	if len(sorted) == 0 {
		return []client.Suggestion{}, nil
	}
	capHint := len(sorted)
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	results := make([]client.Suggestion, 0, capHint)
	for _, sug := range sorted {
		if changeID != "" && sug.ChangeID != changeID {
			continue
		}
		if status != "" && sug.Status != status {
			continue
		}
		results = append(results, sug)
		if limit > 0 && len(results) >= limit {
			return results, nil
		}
	}
	return results, nil
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

const suggestionsCacheVersion = 1

type suggestionsCache struct {
	Version     int                 `json:"version"`
	Suggestions []client.Suggestion `json:"suggestions"`
	RefTip      string              `json:"ref_tip"`
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

func listSuggestionsSorted() ([]client.Suggestion, error) {
	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return nil, err
	}
	refTip := ""
	if resolved, err := gitutil.ResolveRef(notes.RefSuggestions); err == nil {
		refTip = strings.TrimSpace(resolved)
	}
	if refTip != "" {
		if cached, ok := readSuggestionsCache(repoRoot, refTip); ok {
			return cached, nil
		}
	}
	entries, err := loadSuggestionEntries()
	if err != nil {
		return nil, err
	}
	results := make([]client.Suggestion, 0, len(entries))
	for _, entry := range entries {
		results = append(results, entry.Suggestion)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	if refTip != "" {
		writeSuggestionsCache(repoRoot, refTip, results)
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

func suggestionsCachePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".jul", "cache", "suggestions.json")
}

func readSuggestionsCache(repoRoot, refTip string) ([]client.Suggestion, bool) {
	path := suggestionsCachePath(strings.TrimSpace(repoRoot))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cache suggestionsCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if cache.Version != suggestionsCacheVersion {
		return nil, false
	}
	if strings.TrimSpace(cache.RefTip) != strings.TrimSpace(refTip) {
		return nil, false
	}
	return cache.Suggestions, true
}

func writeSuggestionsCache(repoRoot, refTip string, suggestions []client.Suggestion) {
	cache := suggestionsCache{
		Version:     suggestionsCacheVersion,
		Suggestions: suggestions,
		RefTip:      strings.TrimSpace(refTip),
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	path := suggestionsCachePath(strings.TrimSpace(repoRoot))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
