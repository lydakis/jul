package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/metadata"
)

type logEntry struct {
	CommitSHA         string `json:"commit_sha"`
	ChangeID          string `json:"change_id"`
	Author            string `json:"author"`
	Message           string `json:"message"`
	When              string `json:"when"`
	AttestationStatus string `json:"attestation_status,omitempty"`
	Suggestions       int    `json:"suggestions,omitempty"`
}

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

			filtered := make([]logEntry, 0, len(entries))
			for _, cp := range entries {
				if *changeID != "" && cp.ChangeID != *changeID {
					continue
				}
				att, _ := metadata.GetAttestation(cp.SHA)
				suggestions, _ := metadata.ListSuggestions(cp.ChangeID, "open", 1000)
				entry := logEntry{
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

			if len(filtered) == 0 {
				fmt.Fprintln(os.Stdout, "No checkpoints.")
				return 0
			}
			for _, entry := range filtered {
				fmt.Fprintf(os.Stdout, "%s (%s) %q\n", entry.CommitSHA, entry.When, entry.Message)
				if entry.Author != "" {
					fmt.Fprintf(os.Stdout, "        Author: %s\n", entry.Author)
				}
				if entry.AttestationStatus != "" {
					fmt.Fprintf(os.Stdout, "        ✓ CI %s\n", strings.ToLower(entry.AttestationStatus))
				}
				if entry.Suggestions > 0 {
					fmt.Fprintf(os.Stdout, "        %d suggestion(s) pending\n", entry.Suggestions)
				}
				fmt.Fprintln(os.Stdout, "")
			}
			return 0
		},
	}
}
