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
			if len(results) == 0 {
				fmt.Fprintln(os.Stdout, "No suggestions.")
				return 0
			}
			if currentChangeID != "" {
				header := "Pending"
				if statusFilter != "" && statusFilter != "pending" && statusFilter != "stale" {
					header = strings.ToUpper(statusFilter[:1]) + statusFilter[1:]
				}
				if currentCheckpointSHA != "" && currentMessage != "" {
					fmt.Fprintf(os.Stdout, "%s for %s (%s) %q:\n\n", header, currentChangeID, currentCheckpointSHA, currentMessage)
				} else {
					fmt.Fprintf(os.Stdout, "%s for %s:\n\n", header, currentChangeID)
				}
			}
			for _, sug := range results {
				confidence := formatConfidence(sug.Confidence)
				stale := currentCheckpointSHA != "" && sug.BaseCommitSHA != "" && sug.BaseCommitSHA != currentCheckpointSHA
				staleMark := "✓"
				if stale {
					staleMark = "⚠ stale"
				}
				fmt.Fprintf(os.Stdout, "[%s] %s %s %s\n", sug.SuggestionID, sug.Reason, confidence, staleMark)
				if stale && currentCheckpointSHA != "" {
					fmt.Fprintf(os.Stdout, "             Created for %s, current is %s\n", sug.BaseCommitSHA, currentCheckpointSHA)
				} else if sug.BaseCommitSHA != "" {
					fmt.Fprintf(os.Stdout, "             base %s\n", sug.BaseCommitSHA)
				}
				if sug.Description != "" {
					fmt.Fprintf(os.Stdout, "             %s\n", sug.Description)
				}
			}
			fmt.Fprintln(os.Stdout, "\nActions:")
			fmt.Fprintln(os.Stdout, "  jul show <id>      Show diff")
			fmt.Fprintln(os.Stdout, "  jul apply <id>     Apply to draft")
			fmt.Fprintln(os.Stdout, "  jul reject <id>    Reject")
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

			fmt.Fprintf(os.Stdout, "suggestion %s created\n", created.SuggestionID)
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

			fmt.Fprintf(os.Stdout, "%s %s\n", name, updated.SuggestionID)
			return 0
		},
	}
}

func formatConfidence(value float64) string {
	if value <= 0 {
		return ""
	}
	percent := value
	if value <= 1 {
		percent = value * 100
	}
	return fmt.Sprintf("(%.0f%%)", percent)
}
