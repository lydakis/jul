package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

func newSuggestionsCommand() Command {
	return Command{
		Name:    "suggestions",
		Summary: "List suggestions",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("suggestions", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			changeID := fs.String("change-id", "", "Filter by change ID")
			status := fs.String("status", "pending", "Filter by status (pending|applied|rejected|stale|all)")
			limit := fs.Int("limit", 20, "Max results")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			currentChangeID := strings.TrimSpace(*changeID)
			draftSHA := ""
			parentSHA := ""
			baseSHA := ""
			currentMessage := ""
			if draft, parent, err := currentDraftAndBase(); err == nil {
				draftSHA = draft
				parentSHA = parent
				baseSHA = draftSHA
				if checkpoint, _ := latestCheckpoint(); checkpoint != nil && strings.TrimSpace(parentSHA) != "" && checkpoint.SHA == parentSHA {
					baseSHA = parentSHA
				}
				if msg, err := gitutil.CommitMessage(baseSHA); err == nil {
					currentMessage = firstLine(msg)
				}
			}
			if currentChangeID == "" {
				if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
					currentChangeID = checkpoint.ChangeID
				}
			}

			statusFilter := strings.TrimSpace(*status)
			switch statusFilter {
			case "open":
				statusFilter = "pending"
			case "accepted":
				statusFilter = "applied"
			}
			listStatus := statusFilter
			if statusFilter == "all" {
				listStatus = ""
			}
			if statusFilter == "stale" {
				listStatus = "pending"
			}
			results, err := metadata.ListSuggestions(currentChangeID, listStatus, *limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to list suggestions: %v\n", err)
				return 1
			}
			if statusFilter == "stale" {
				staleOnly := make([]client.Suggestion, 0, len(results))
				for _, sug := range results {
					if suggestionIsStale(sug.BaseCommitSHA, draftSHA, parentSHA) {
						staleOnly = append(staleOnly, sug)
					}
				}
				results = staleOnly
			}
			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(results); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}
			output.RenderSuggestions(os.Stdout, output.SuggestionsView{
				ChangeID:          currentChangeID,
				Status:            statusFilter,
				CheckpointSHA:     baseSHA,
				CheckpointMessage: currentMessage,
				Suggestions:       results,
			}, output.DefaultOptions())
			return 0
		},
	}
}

func newSuggestionActionCommand(name, action string) Command {
	return Command{
		Name:    name,
		Summary: fmt.Sprintf("%s a suggestion", strings.ToUpper(action[:1])+action[1:]),
		Run: func(args []string) int {
			fs := flag.NewFlagSet(name, flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			message := fs.String("m", "", "Resolution note")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)
			id := strings.TrimSpace(fs.Arg(0))
			if id == "" {
				fmt.Fprintf(os.Stderr, "%s id required\n", name)
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
				fmt.Fprintf(os.Stderr, "failed to update suggestion: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(updated); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			output.RenderSuggestionUpdated(os.Stdout, name, updated)
			return 0
		},
	}
}
