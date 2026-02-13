package cli

import (
	"path/filepath"
	"strings"
	"time"

	cicmd "github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/metrics"
	"github.com/lydakis/jul/cli/internal/output"
	wsconfig "github.com/lydakis/jul/cli/internal/workspace"
)

type workingTreeResult struct {
	tree     *output.WorkingTreeStatus
	err      error
	duration time.Duration
}

func buildLocalStatus() (output.Status, error) {
	timings := metrics.NewTimings()
	workingTreeCh := make(chan workingTreeResult, 1)
	go func() {
		start := time.Now()
		tree, err := readWorkingTreeStatus()
		workingTreeCh <- workingTreeResult{
			tree:     tree,
			err:      err,
			duration: time.Since(start),
		}
	}()

	workspaceStart := time.Now()
	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	wsID := config.WorkspaceID()
	timings.Add("workspace", time.Since(workspaceStart))

	repoRootStart := time.Now()
	repoRoot, _ := gitutil.RepoTopLevel()
	repoRoot = strings.TrimSpace(repoRoot)
	timings.Add("repo_root", time.Since(repoRootStart))

	var cached *statusCache
	if repoRoot != "" {
		cacheStart := time.Now()
		if cache, err := readStatusCache(repoRoot); err == nil && cache != nil {
			if cacheMatchesWorkspace(cache, wsID, workspace) {
				cached = cache
			}
		}
		if cached == nil {
			if refreshed, err := refreshStatusCache(repoRoot); err == nil && refreshed != nil {
				if cacheMatchesWorkspace(refreshed, wsID, workspace) {
					cached = refreshed
				}
			}
		}
		timings.Add("read_cache", time.Since(cacheStart))
	}

	commitStart := time.Now()
	info := gitutil.CommitInfo{}
	if cached != nil && strings.TrimSpace(cached.Branch) != "" {
		info.Branch = strings.TrimSpace(cached.Branch)
		info.RepoName = strings.TrimSpace(cached.Repo)
		info.TopLevel = repoRoot
		info.SHA = strings.TrimSpace(cached.DraftSHA)
		info.ChangeID = strings.TrimSpace(cached.ChangeID)
	} else {
		head, err := gitutil.ReadHeadInfo()
		if err == nil {
			info.SHA = head.SHA
			info.Branch = head.Branch
			info.TopLevel = head.RepoRoot
		} else {
			info, err = gitutil.CurrentCommit()
			if err != nil {
				info, err = fallbackCommitInfo()
				if err != nil {
					return output.Status{}, err
				}
			}
		}
	}
	timings.Add("commit_info", time.Since(commitStart))

	repoName := strings.TrimSpace(info.RepoName)
	if repoName == "" {
		repoName = config.RepoName()
	}
	if repoName == "" && repoRoot != "" {
		repoName = filepath.Base(repoRoot)
	}
	info.RepoName = strings.TrimSpace(repoName)
	if strings.TrimSpace(info.TopLevel) == "" {
		info.TopLevel = repoRoot
	}

	draftStart := time.Now()
	draftSHA := strings.TrimSpace(info.SHA)
	if cached != nil && strings.TrimSpace(cached.DraftSHA) != "" {
		draftSHA = strings.TrimSpace(cached.DraftSHA)
	} else if ref, err := syncRef(user, workspace); err == nil {
		if sha, err := gitutil.ResolveRef(ref); err == nil {
			draftSHA = strings.TrimSpace(sha)
		}
	}
	timings.Add("draft_sha", time.Since(draftStart))

	var checkpoint *output.CheckpointStatus
	var attView attestationView
	attStart := time.Now()
	attResolved := false
	if cached != nil && cached.LastCheckpoint != nil {
		checkpoint = cached.LastCheckpoint
		if cachedView, ok := cachedAttestationView(cached, checkpoint.CommitSHA); ok {
			attView = cachedView
			attResolved = true
		} else if checkpoint.CommitSHA != "" {
			attView, _ = resolveAttestationView(checkpoint.CommitSHA)
			if attView.Status != "" || attView.Attestation != nil || attView.InheritedFrom != "" {
				attResolved = true
			}
		}
	} else {
		last, err := latestCheckpoint()
		if err != nil {
			return output.Status{}, err
		}
		if last != nil {
			checkpoint = &output.CheckpointStatus{
				CommitSHA: last.SHA,
				Message:   firstLine(last.Message),
				Author:    last.Author,
				When:      last.When.Format("2006-01-02 15:04:05"),
				ChangeID:  last.ChangeID,
			}
			attView, _ = resolveAttestationView(last.SHA)
			if attView.Status != "" || attView.Attestation != nil || attView.InheritedFrom != "" {
				attResolved = true
			}
		}
	}
	if !attResolved && draftSHA != "" {
		if checkpoint == nil || checkpoint.CommitSHA != draftSHA {
			attView, _ = resolveAttestationView(draftSHA)
		}
	}
	if !attResolved && info.SHA != "" && info.SHA != draftSHA {
		attView, _ = resolveAttestationView(info.SHA)
	}
	timings.Add("attestation", time.Since(attStart))

	changeID := ""
	if cached != nil && strings.TrimSpace(cached.ChangeID) != "" {
		changeID = strings.TrimSpace(cached.ChangeID)
	}
	if checkpoint != nil && checkpoint.ChangeID != "" {
		changeID = checkpoint.ChangeID
	}
	draftChangeID := ""
	if changeID == "" && draftSHA != "" {
		if msg, err := gitutil.CommitMessage(draftSHA); err == nil {
			draftChangeID = gitutil.ExtractChangeID(msg)
		}
		if draftChangeID == "" {
			draftChangeID = gitutil.FallbackChangeID(draftSHA)
		}
		changeID = draftChangeID
	}
	if changeID == "" && info.ChangeID != "" {
		changeID = info.ChangeID
	}
	if changeID == "" {
		if draftSHA != "" {
			changeID = gitutil.FallbackChangeID(draftSHA)
		} else {
			changeID = gitutil.FallbackChangeID(info.SHA)
		}
	}

	suggestStart := time.Now()
	suggestionsPending := 0
	if cached != nil {
		if cached.SuggestionsPendingByChange != nil {
			suggestionsPending = cached.SuggestionsPendingByChange[changeID]
		}
	} else {
		suggestions, err := metadata.ListSuggestions(changeID, "pending", 1000)
		if err != nil {
			return output.Status{}, err
		}
		suggestionsPending = len(suggestions)
	}
	timings.Add("suggestions_pending", time.Since(suggestStart))

	status := output.Status{
		WorkspaceID:        wsID,
		Workspace:          workspace,
		WorkspaceDefault:   workspace == config.WorkspaceName(),
		Repo:               info.RepoName,
		Branch:             info.Branch,
		DraftSHA:           draftSHA,
		ChangeID:           changeID,
		SyncStatus:         "local",
		LastCheckpoint:     checkpoint,
		SuggestionsPending: suggestionsPending,
	}
	if repoRoot != "" {
		trackStart := time.Now()
		if cfg, ok, err := wsconfig.ReadConfig(repoRoot, workspace); err == nil && ok {
			status.TrackRef = strings.TrimSpace(cfg.TrackRef)
			status.TrackTip = strings.TrimSpace(cfg.TrackTip)
			if status.TrackRef != "" && cached != nil {
				status.TrackTipCurrent = status.TrackTip
			}
			if status.TrackRef != "" && cached == nil {
				if tip, err := gitutil.ResolveRef(status.TrackRef); err == nil {
					status.TrackTipCurrent = strings.TrimSpace(tip)
					if status.TrackTip != "" && status.TrackTipCurrent != "" && status.TrackTipCurrent != status.TrackTip {
						status.BaseAdvanced = true
					}
				}
			}
		}
		timings.Add("track_ref", time.Since(trackStart))
	}
	if attView.Status != "" {
		status.AttestationStatus = attView.Status
		status.AttestationStale = attView.Stale
		status.AttestationInheritedFrom = attView.InheritedFrom
	}
	if repoRoot != "" {
		if run, ok := readSyncRunning(repoRoot); ok && run != nil && syncRunActive(*run) {
			status.SyncState = "running"
		} else {
			status.SyncState = "idle"
		}
	}

	filesStart := time.Now()
	filesChanged := 0
	if cached != nil {
		filesChanged = cached.DraftFilesChanged
	} else {
		baseSHA := ""
		if checkpoint != nil && checkpoint.CommitSHA != "" {
			baseSHA = checkpoint.CommitSHA
		}
		filesChanged = draftFilesChangedFrom(baseSHA, draftSHA)
	}
	timings.Add("draft_files", time.Since(filesStart))
	status.Draft = &output.DraftStatus{
		CommitSHA:    draftSHA,
		ChangeID:     changeID,
		FilesChanged: filesChanged,
	}
	draftCIStart := time.Now()
	status.DraftCI = buildDraftCIStatus(draftSHA)
	timings.Add("draft_ci", time.Since(draftCIStart))
	workRes := <-workingTreeCh
	if workRes.err == nil {
		status.WorkingTree = workRes.tree
	}
	timings.Add("working_tree", workRes.duration)

	if cached != nil && len(cached.Checkpoints) > 0 {
		status.Checkpoints = cached.Checkpoints
		if cached.PromoteStatus != nil {
			status.PromoteStatus = cached.PromoteStatus
		} else {
			promoteStart := time.Now()
			status.PromoteStatus = buildPromoteStatusFromSummaries(cached.Checkpoints)
			timings.Add("promote_status", time.Since(promoteStart))
		}
	} else {
		fallbackStart := time.Now()
		checkpoints, err := listCheckpoints(statusCacheLimit)
		if err != nil {
			return output.Status{}, err
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
		status.Checkpoints = summaries
		promoteStart := time.Now()
		status.PromoteStatus = buildPromoteStatusFromSummaries(summaries)
		timings.Add("promote_status", time.Since(promoteStart))
		timings.Add("fallback_scan", time.Since(fallbackStart))
	}
	status.Timings = timings
	return status, nil
}

func buildPromoteStatusFromSummaries(checkpoints []output.CheckpointSummary) *output.PromoteStatus {
	if len(checkpoints) == 0 {
		return buildPromoteStatus(nil)
	}
	entries := make([]checkpointInfo, 0, len(checkpoints))
	for _, cp := range checkpoints {
		entries = append(entries, checkpointInfo{
			SHA:      cp.CommitSHA,
			Message:  cp.Message,
			ChangeID: cp.ChangeID,
		})
	}
	return buildPromoteStatus(entries)
}

func cachedAttestationView(cache *statusCache, commitSHA string) (attestationView, bool) {
	if cache == nil || strings.TrimSpace(commitSHA) == "" {
		return attestationView{}, false
	}
	for _, summary := range cache.Checkpoints {
		if summary.CommitSHA != commitSHA {
			continue
		}
		return attestationView{
			Status:        summary.CIStatus,
			Stale:         summary.CIStale,
			InheritedFrom: summary.CIInheritedFrom,
		}, true
	}
	return attestationView{}, false
}

func buildDraftCIStatus(draftSHA string) *output.CIStatusDetails {
	completed, err := cicmd.ReadCompleted()
	if err != nil {
		return nil
	}
	running, _ := cicmd.ReadRunning()
	if completed == nil && running == nil {
		return nil
	}
	configured := hasCIConfig()
	if !configured {
		if root, err := gitutil.RepoTopLevel(); err == nil {
			configured = hasCIInference(root)
		}
	}
	status := "unknown"
	resultsCurrent := false
	if completed != nil {
		resultsCurrent = completed.CommitSHA == draftSHA
		if resultsCurrent {
			status = completed.Result.Status
		} else {
			status = "stale"
		}
	}
	if running != nil && running.CommitSHA == draftSHA {
		status = "running"
	}
	if completed != nil && hasRealCIResult(completed.Result.Commands) {
		configured = true
	}
	if running != nil && running.CommitSHA == draftSHA {
		configured = true
	}
	if !configured && !resultsCurrent && (running == nil || running.CommitSHA != draftSHA) {
		return nil
	}
	details := &output.CIStatusDetails{
		Status:          status,
		CurrentDraftSHA: draftSHA,
		ResultsCurrent:  resultsCurrent,
	}
	if completed != nil {
		details.CompletedSHA = completed.CommitSHA
		if !completed.Result.StartedAt.IsZero() && !completed.Result.FinishedAt.IsZero() {
			details.DurationMs = completed.Result.FinishedAt.Sub(completed.Result.StartedAt).Milliseconds()
		}
		checks := make([]output.CICheck, 0, len(completed.Result.Commands))
		for _, cmd := range completed.Result.Commands {
			checks = append(checks, output.CICheck{
				Name:       output.LabelForCommand(cmd.Command),
				Status:     cmd.Status,
				DurationMs: cmd.DurationMs,
				Output:     cmd.OutputExcerpt,
			})
		}
		if completed.CoverageLinePct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_line",
				Status: "pass",
				Value:  *completed.CoverageLinePct,
			})
		}
		if completed.CoverageBranchPct != nil {
			checks = append(checks, output.CICheck{
				Name:   "coverage_branch",
				Status: "pass",
				Value:  *completed.CoverageBranchPct,
			})
		}
		details.Results = checks
	}
	if running != nil {
		details.RunningSHA = running.CommitSHA
		details.RunningPID = running.PID
	}
	return details
}

func hasCIConfig() bool {
	cfg, ok, err := cicmd.LoadConfig()
	if err != nil || !ok || len(cfg.Commands) == 0 {
		return false
	}
	for _, cmd := range cfg.Commands {
		if isRealCICommand(cmd.Command) {
			return true
		}
	}
	return false
}

func hasCIInference(root string) bool {
	cmds := cicmd.InferDefaultCommands(root)
	for _, cmd := range cmds {
		if isRealCICommand(cmd) {
			return true
		}
	}
	return false
}

func hasRealCIResult(cmds []cicmd.CommandResult) bool {
	for _, cmd := range cmds {
		if isRealCICommand(cmd.Command) {
			return true
		}
	}
	return false
}

func isRealCICommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return false
	}
	if strings.EqualFold(trimmed, "true") {
		return false
	}
	return true
}

func buildPromoteStatus(checkpoints []checkpointInfo) *output.PromoteStatus {
	if len(checkpoints) == 0 {
		return nil
	}
	target := config.PromoteTarget()
	if target == "" {
		target = "main"
	}
	targetRef := "refs/heads/" + target
	if !gitutil.RefExists(targetRef) {
		return &output.PromoteStatus{
			Target:           target,
			Eligible:         false,
			CheckpointsAhead: len(checkpoints),
		}
	}
	ahead := 0
	for _, cp := range checkpoints {
		_, err := gitutil.Git("merge-base", "--is-ancestor", cp.SHA, targetRef)
		if err != nil {
			ahead++
		}
	}
	return &output.PromoteStatus{
		Target:           target,
		Eligible:         true,
		CheckpointsAhead: ahead,
	}
}

func readWorkingTreeStatus() (*output.WorkingTreeStatus, error) {
	out, err := gitutil.Git(
		"--no-optional-locks",
		"-c", "core.untrackedCache=true",
		"status",
		"--porcelain",
		"-z",
		"-unormal",
		"--no-renames",
	)
	if err != nil {
		return nil, err
	}
	return parseWorkingTreePorcelainZ(out), nil
}

func parseWorkingTreePorcelainZ(raw string) *output.WorkingTreeStatus {
	status := &output.WorkingTreeStatus{}
	if strings.TrimSpace(raw) != "" {
		for _, entry := range strings.Split(raw, "\x00") {
			if len(entry) < 4 || entry[2] != ' ' {
				continue
			}
			code := entry[:2]
			path := entry[3:]
			if path == "" {
				continue
			}
			if code == "??" {
				status.Untracked = append(status.Untracked, output.WorkingTreeEntry{Path: path, Status: "?"})
				continue
			}
			staged := code[0]
			unstaged := code[1]
			if staged != ' ' {
				status.Staged = append(status.Staged, output.WorkingTreeEntry{Path: path, Status: string(staged)})
			}
			if unstaged != ' ' && unstaged != '?' {
				status.Unstaged = append(status.Unstaged, output.WorkingTreeEntry{Path: path, Status: string(unstaged)})
			}
		}
	}
	if len(status.Staged) == 0 && len(status.Unstaged) == 0 && len(status.Untracked) == 0 {
		status.Clean = true
	}
	return status
}

func fallbackCommitInfo() (gitutil.CommitInfo, error) {
	sha, err := currentDraftSHA()
	if err != nil || strings.TrimSpace(sha) == "" {
		return gitutil.CommitInfo{}, err
	}
	message, _ := gitutil.CommitMessage(sha)
	author, _ := gitutil.Git("log", "-1", "--format=%an", sha)
	committedISO, _ := gitutil.Git("log", "-1", "--format=%cI", sha)
	top, _ := gitutil.RepoTopLevel()

	committed := time.Now().UTC()
	if committedISO != "" {
		if parsed, err := time.Parse(time.RFC3339, committedISO); err == nil {
			committed = parsed
		}
	}

	changeID := gitutil.ExtractChangeID(message)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(sha)
	}

	repoName := ""
	if top != "" {
		repoName = filepath.Base(top)
	}

	return gitutil.CommitInfo{
		SHA:       strings.TrimSpace(sha),
		Author:    strings.TrimSpace(author),
		Message:   message,
		Committed: committed,
		RepoName:  repoName,
		ChangeID:  changeID,
		TopLevel:  top,
	}, nil
}
