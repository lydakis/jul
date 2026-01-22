package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
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

			currentCheckpoint, _ := latestCheckpoint()
			currentChangeID := strings.TrimSpace(*changeID)
			currentCheckpointSHA := ""
			currentMessage := ""
			if currentCheckpoint != nil {
				currentCheckpointSHA = currentCheckpoint.SHA
				currentMessage = firstLine(currentCheckpoint.Message)
				if currentChangeID == "" {
					currentChangeID = currentCheckpoint.ChangeID
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
					stale := currentCheckpointSHA != "" && sug.BaseCommitSHA != "" && sug.BaseCommitSHA != currentCheckpointSHA
					if stale {
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
				CheckpointSHA:     currentCheckpointSHA,
				CheckpointMessage: currentMessage,
				Suggestions:       results,
			})
			return 0
		},
	}
}

func newSuggestCommand() Command {
	return Command{
		Name:    "suggest",
		Summary: "Create a suggestion for the current change",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("suggest", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			baseSHA := fs.String("base", "", "Base commit SHA (default: HEAD)")
			suggestedSHA := fs.String("suggested", "", "Suggested commit SHA")
			reason := fs.String("reason", "unspecified", "Suggestion reason")
			description := fs.String("description", "", "Suggestion description")
			confidence := fs.Float64("confidence", 0, "Suggestion confidence")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			if strings.TrimSpace(*suggestedSHA) == "" {
				fmt.Fprintln(os.Stderr, "--suggested is required")
				return 1
			}

			base := strings.TrimSpace(*baseSHA)
			if base == "" {
				base = "HEAD"
			}
			baseResolved, err := gitutil.Git("rev-parse", base)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to resolve base: %v\n", err)
				return 1
			}

			created, err := metadata.CreateSuggestion(metadata.SuggestionCreate{
				ChangeID:           "",
				BaseCommitSHA:      strings.TrimSpace(baseResolved),
				SuggestedCommitSHA: strings.TrimSpace(*suggestedSHA),
				CreatedBy:          config.UserName(),
				Reason:             strings.TrimSpace(*reason),
				Description:        strings.TrimSpace(*description),
				Confidence:         *confidence,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create suggestion: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(created); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			output.RenderSuggestionCreated(os.Stdout, created)
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
