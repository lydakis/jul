package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

func newLogCommand() Command {
	return Command{
		Name:    "log",
		Summary: "Show checkpoint history",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("log", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			limit := fs.Int("limit", 20, "Max checkpoints to show")
			changeID := fs.String("change-id", "", "Filter by Change-Id")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			entries, err := listCheckpoints()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to list checkpoints: %v\n", err)
				return 1
			}

			filtered := make([]output.LogEntry, 0, len(entries))
			for _, cp := range entries {
				if *changeID != "" && cp.ChangeID != *changeID {
					continue
				}
				att, _ := metadata.GetAttestation(cp.SHA)
				suggestions, _ := metadata.ListSuggestions(cp.ChangeID, "pending", 1000)
				entry := output.LogEntry{
					CommitSHA:   cp.SHA,
					ChangeID:    cp.ChangeID,
					Author:      cp.Author,
					Message:     firstLine(cp.Message),
					When:        cp.When.Format("2006-01-02 15:04:05"),
					Suggestions: len(suggestions),
				}
				if att != nil {
					entry.AttestationStatus = att.Status
				}
				filtered = append(filtered, entry)
				if *limit > 0 && len(filtered) >= *limit {
					break
				}
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(filtered); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			output.RenderLog(os.Stdout, filtered, output.DefaultOptions())
			return 0
		},
	}
}
