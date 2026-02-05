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
	"github.com/lydakis/jul/cli/internal/syncer"
)

const statusCacheLimit = 20

type statusCache struct {
	WorkspaceID                string                     `json:"workspace_id,omitempty"`
	Workspace                  string                     `json:"workspace,omitempty"`
	GeneratedAt                time.Time                  `json:"generated_at"`
	LastCheckpoint             *output.CheckpointStatus   `json:"last_checkpoint,omitempty"`
	Checkpoints                []output.CheckpointSummary `json:"checkpoints,omitempty"`
	DraftFilesChanged          int                        `json:"draft_files_changed,omitempty"`
	PromoteStatus              *output.PromoteStatus      `json:"promote_status,omitempty"`
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
	user, workspace := workspaceParts()
	checkpoints, err := listCheckpoints(statusCacheLimit)
	if err != nil {
		return nil, err
	}
	pendingCounts, _ := metadata.PendingSuggestionCounts()
	if pendingCounts == nil {
		pendingCounts = map[string]int{}
	}

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
	promote := buildPromoteStatusFromSummaries(summaries)
	draftSHA := ""
	if ref, err := syncRef(user, workspace); err == nil {
		if sha, err := gitutil.ResolveRef(ref); err == nil {
			draftSHA = sha
		}
	}
	baseSHA := ""
	if last != nil {
		baseSHA = last.CommitSHA
	}
	draftFilesChanged := draftFilesChangedFrom(baseSHA, draftSHA)

	cache := statusCache{
		WorkspaceID:                wsID,
		Workspace:                  workspace,
		GeneratedAt:                time.Now().UTC(),
		LastCheckpoint:             last,
		Checkpoints:                summaries,
		DraftFilesChanged:          draftFilesChanged,
		PromoteStatus:              promote,
		SuggestionsPendingByChange: pendingCounts,
	}
	if err := writeStatusCache(repoRoot, cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

func updateStatusCacheForCheckpoint(repoRoot string, res syncer.CheckpointResult) (*statusCache, error) {
	if strings.TrimSpace(repoRoot) == "" {
		root, err := gitutil.RepoTopLevel()
		if err != nil {
			return nil, err
		}
		repoRoot = root
	}
	cache, err := readStatusCache(repoRoot)
	if err != nil || cache == nil {
		return refreshStatusCache(repoRoot)
	}

	author := strings.TrimSpace(config.UserName())
	if author == "" {
		author = "user"
	}
	when := time.Now().UTC()
	whenLabel := when.Format("2006-01-02 15:04:05")
	message := strings.TrimSpace(res.Message)
	summary := output.CheckpointSummary{
		CommitSHA: res.CheckpointSHA,
		Message:   firstLine(message),
		ChangeID:  res.ChangeID,
		When:      whenLabel,
	}
	last := &output.CheckpointStatus{
		CommitSHA: res.CheckpointSHA,
		Message:   summary.Message,
		Author:    author,
		When:      whenLabel,
		ChangeID:  res.ChangeID,
	}

	updated := make([]output.CheckpointSummary, 0, statusCacheLimit)
	if summary.CommitSHA != "" {
		updated = append(updated, summary)
	}
	for _, cp := range cache.Checkpoints {
		if cp.CommitSHA == "" || cp.CommitSHA == summary.CommitSHA {
			continue
		}
		updated = append(updated, cp)
		if len(updated) >= statusCacheLimit {
			break
		}
	}

	cache.WorkspaceID = strings.TrimSpace(config.WorkspaceID())
	_, workspace := workspaceParts()
	cache.Workspace = strings.TrimSpace(workspace)
	cache.GeneratedAt = time.Now().UTC()
	cache.LastCheckpoint = last
	cache.Checkpoints = updated
	cache.DraftFilesChanged = 0
	if cache.SuggestionsPendingByChange == nil {
		cache.SuggestionsPendingByChange = map[string]int{}
	}
	if res.ChangeID != "" {
		if _, ok := cache.SuggestionsPendingByChange[res.ChangeID]; !ok {
			cache.SuggestionsPendingByChange[res.ChangeID] = 0
		}
	}

	if cache.PromoteStatus != nil && cache.PromoteStatus.Target != "" && cache.PromoteStatus.Eligible {
		cache.PromoteStatus.CheckpointsAhead++
	}

	if err := writeStatusCache(repoRoot, *cache); err != nil {
		return nil, err
	}
	return cache, nil
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
