package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/metrics"
	"github.com/lydakis/jul/cli/internal/output"
)

func newSuggestionsCommand() Command {
	return Command{
		Name:    "suggestions",
		Summary: "List suggestions",
		Run: func(args []string) int {
			totalStart := time.Now()
			timings := metrics.NewTimings()
			fs, jsonOut := newFlagSet("suggestions")
			changeID := fs.String("change-id", "", "Filter by change ID")
			status := fs.String("status", "pending", "Filter by status (pending|applied|rejected|stale|all)")
			limit := fs.Int("limit", 20, "Max results")
			_ = fs.Parse(args)

			contextStart := time.Now()
			statusFilter := strings.TrimSpace(*status)
			switch statusFilter {
			case "open":
				statusFilter = "pending"
			case "accepted":
				statusFilter = "applied"
			}

			currentChangeID := strings.TrimSpace(*changeID)
			draftSHA := ""
			parentSHA := ""
			baseSHA := ""
			currentMessage := ""
			needStaleFilter := statusFilter == "stale" || statusFilter == "pending"
			needContext := needStaleFilter || currentChangeID == "" || !*jsonOut

			if needContext {
				if repoRoot, err := gitutil.RepoTopLevel(); err == nil {
					if cache, err := readStatusCache(repoRoot); err == nil && cache != nil {
						wsID := strings.TrimSpace(config.WorkspaceID())
						_, wsName := workspaceParts()
						if wsName == "" {
							wsName = "@"
						}
						if cacheMatchesWorkspace(cache, wsID, wsName) {
							if draftSHA == "" {
								draftSHA = strings.TrimSpace(cache.DraftSHA)
							}
							if currentChangeID == "" {
								currentChangeID = strings.TrimSpace(cache.ChangeID)
							}
							if cache.LastCheckpoint != nil {
								cpSHA := strings.TrimSpace(cache.LastCheckpoint.CommitSHA)
								if parentSHA == "" {
									parentSHA = cpSHA
								}
								if baseSHA == "" {
									baseSHA = cpSHA
								}
								if !*jsonOut && currentMessage == "" {
									currentMessage = firstLine(cache.LastCheckpoint.Message)
								}
							}
						}
					}
				}
			}

			needLiveContext := (needStaleFilter && (strings.TrimSpace(draftSHA) == "" || strings.TrimSpace(parentSHA) == "")) ||
				currentChangeID == "" ||
				(!*jsonOut && (strings.TrimSpace(baseSHA) == "" || currentMessage == ""))
			if needLiveContext {
				if draft, parent, err := currentDraftAndBase(); err == nil {
					draftSHA = draft
					parentSHA = parent
					baseSHA = parentSHA
					if strings.TrimSpace(baseSHA) == "" {
						baseSHA = draftSHA
					}
					if currentChangeID == "" {
						if msg, err := gitutil.CommitMessage(draftSHA); err == nil {
							currentChangeID = strings.TrimSpace(gitutil.ExtractChangeID(msg))
						}
					}
					if !*jsonOut && strings.TrimSpace(baseSHA) != "" {
						if msg, err := gitutil.CommitMessage(baseSHA); err == nil {
							currentMessage = firstLine(msg)
						}
					}
				}
			}

			if currentChangeID == "" {
				if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
					currentChangeID = checkpoint.ChangeID
					if strings.TrimSpace(baseSHA) == "" {
						baseSHA = checkpoint.SHA
					}
					if !*jsonOut && currentMessage == "" {
						currentMessage = firstLine(checkpoint.Message)
					}
				}
			}
			timings.Add("context", time.Since(contextStart))

			listStart := time.Now()
			listStatus := statusFilter
			if statusFilter == "all" {
				listStatus = ""
			}
			if statusFilter == "stale" {
				listStatus = "pending"
			}
			results, err := metadata.ListSuggestions(currentChangeID, listStatus, *limit)
			timings.Add("list", time.Since(listStart))
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "suggestions_list_failed", fmt.Sprintf("failed to list suggestions: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to list suggestions: %v\n", err)
				}
				return 1
			}
			filterStart := time.Now()
			if statusFilter == "stale" || statusFilter == "pending" {
				staleOnly := make([]client.Suggestion, 0, len(results))
				freshOnly := make([]client.Suggestion, 0, len(results))
				for _, sug := range results {
					isStale := suggestionIsStale(sug.BaseCommitSHA, draftSHA, parentSHA)
					if isStale {
						staleOnly = append(staleOnly, sug)
					} else {
						freshOnly = append(freshOnly, sug)
					}
				}
				if statusFilter == "stale" {
					results = staleOnly
				} else {
					results = freshOnly
				}
			}
			timings.Add("filter", time.Since(filterStart))
			view := output.SuggestionsView{
				ChangeID:          currentChangeID,
				Status:            statusFilter,
				CheckpointSHA:     baseSHA,
				CheckpointMessage: currentMessage,
				Suggestions:       results,
				Timings:           timings,
			}
			view.Timings.TotalMs = time.Since(totalStart).Milliseconds()
			if *jsonOut {
				return writeJSON(view)
			}
			output.RenderSuggestions(os.Stdout, view, output.DefaultOptions())
			return 0
		},
	}
}

func newSuggestionActionCommand(name, action string) Command {
	return Command{
		Name:    name,
		Summary: fmt.Sprintf("%s a suggestion", strings.ToUpper(action[:1])+action[1:]),
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet(name)
			message := fs.String("m", "", "Resolution note")
			_ = fs.Parse(args)
			id := strings.TrimSpace(fs.Arg(0))
			if id == "" {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "suggestion_missing_id", fmt.Sprintf("%s id required", name), nil)
				} else {
					fmt.Fprintf(os.Stderr, "%s id required\n", name)
				}
				return 1
			}

			status := action
			if action == "accept" {
				status = "applied"
			}
			if action == "reject" {
				status = "rejected"
			}
			updated, err := metadata.UpdateSuggestionStatus(id, status, *message)
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "suggestion_update_failed", fmt.Sprintf("failed to update suggestion: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to update suggestion: %v\n", err)
				}
				return 1
			}

			if *jsonOut {
				return writeJSON(updated)
			}

			output.RenderSuggestionUpdated(os.Stdout, name, updated)
			return 0
		},
	}
}
