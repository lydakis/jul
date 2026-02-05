package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

const statusCacheLimit = 20

type statusCache struct {
	WorkspaceID                string                     `json:"workspace_id,omitempty"`
	Workspace                  string                     `json:"workspace,omitempty"`
	GeneratedAt                time.Time                  `json:"generated_at"`
	LastCheckpoint             *output.CheckpointStatus   `json:"last_checkpoint,omitempty"`
	Checkpoints                []output.CheckpointSummary `json:"checkpoints,omitempty"`
	SuggestionsPendingByChange map[string]int             `json:"suggestions_pending_by_change,omitempty"`
}

func readStatusCache(repoRoot string) (*statusCache, error) {
	path := statusCachePath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache statusCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

func writeStatusCache(repoRoot string, cache statusCache) error {
	path := statusCachePath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func statusCachePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".jul", "status.json")
}

func refreshStatusCache(repoRoot string) (*statusCache, error) {
	if strings.TrimSpace(repoRoot) == "" {
		root, err := gitutil.RepoTopLevel()
		if err != nil {
			return nil, err
		}
		repoRoot = root
	}
	wsID := strings.TrimSpace(config.WorkspaceID())
	_, workspace := workspaceParts()
	checkpoints, err := listCheckpoints(statusCacheLimit)
	if err != nil {
		return nil, err
	}
	pendingCounts, _ := metadata.PendingSuggestionCounts()

	summaries := make([]output.CheckpointSummary, 0, len(checkpoints))
	for _, cp := range checkpoints {
		ciView, _ := resolveAttestationView(cp.SHA)
		count := 0
		if pendingCounts != nil {
			count = pendingCounts[cp.ChangeID]
		}
		summaries = append(summaries, output.CheckpointSummary{
			CommitSHA:          cp.SHA,
			Message:            firstLine(cp.Message),
			ChangeID:           cp.ChangeID,
			When:               cp.When.Format("2006-01-02 15:04:05"),
			CIStatus:           ciView.Status,
			CIStale:            ciView.Stale,
			CIInheritedFrom:    ciView.InheritedFrom,
			SuggestionsPending: count,
		})
	}

	var last *output.CheckpointStatus
	if len(checkpoints) > 0 {
		cp := checkpoints[0]
		last = &output.CheckpointStatus{
			CommitSHA: cp.SHA,
			Message:   firstLine(cp.Message),
			Author:    cp.Author,
			When:      cp.When.Format("2006-01-02 15:04:05"),
			ChangeID:  cp.ChangeID,
		}
	}

	cache := statusCache{
		WorkspaceID:                wsID,
		Workspace:                  workspace,
		GeneratedAt:                time.Now().UTC(),
		LastCheckpoint:             last,
		Checkpoints:                summaries,
		SuggestionsPendingByChange: pendingCounts,
	}
	if err := writeStatusCache(repoRoot, cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

func cacheMatchesWorkspace(cache *statusCache, wsID, workspace string) bool {
	if cache == nil {
		return false
	}
	wsID = strings.TrimSpace(wsID)
	workspace = strings.TrimSpace(workspace)
	if cache.WorkspaceID != "" {
		return strings.TrimSpace(cache.WorkspaceID) == wsID && wsID != ""
	}
	if cache.Workspace != "" {
		return strings.TrimSpace(cache.Workspace) == workspace && workspace != ""
	}
	return false
}
