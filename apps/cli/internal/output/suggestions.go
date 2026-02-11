package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/metrics"
)

type SuggestionsView struct {
	ChangeID          string              `json:"change_id,omitempty"`
	Status            string              `json:"status,omitempty"`
	CheckpointSHA     string              `json:"checkpoint_sha,omitempty"`
	CheckpointMessage string              `json:"checkpoint_message,omitempty"`
	Suggestions       []client.Suggestion `json:"suggestions"`
	Timings           metrics.Timings     `json:"timings_ms,omitempty"`
}

func RenderSuggestions(w io.Writer, view SuggestionsView, opts Options) {
	if len(view.Suggestions) == 0 {
		fmt.Fprintln(w, "No suggestions.")
		return
	}
	if view.ChangeID != "" {
		header := "Pending"
		if view.Status != "" && view.Status != "pending" && view.Status != "stale" {
			header = strings.ToUpper(view.Status[:1]) + view.Status[1:]
		}
		if view.CheckpointSHA != "" && view.CheckpointMessage != "" {
			fmt.Fprintf(w, "%s for %s (%s) %q:\n\n", header, view.ChangeID, view.CheckpointSHA, view.CheckpointMessage)
		} else {
			fmt.Fprintf(w, "%s for %s:\n\n", header, view.ChangeID)
		}
	}
	passMark := statusIconColored("pass", opts)
	if passMark == "" {
		passMark = statusIcon("pass", opts)
	}
	warnMark := statusIconColored("warning", opts)
	if warnMark == "" {
		warnMark = statusIcon("warning", opts)
	}
	for _, sug := range view.Suggestions {
		confidence := formatConfidence(sug.Confidence)
		stale := view.CheckpointSHA != "" && sug.BaseCommitSHA != "" && sug.BaseCommitSHA != view.CheckpointSHA
		staleMark := strings.TrimSpace(passMark)
		if stale {
			staleMark = strings.TrimSpace(warnMark) + " stale"
		}
		fmt.Fprintf(w, "[%s] %s %s %s\n", sug.SuggestionID, sug.Reason, confidence, staleMark)
		if stale && view.CheckpointSHA != "" {
			fmt.Fprintf(w, "             Created for %s, current is %s\n", sug.BaseCommitSHA, view.CheckpointSHA)
		} else if sug.BaseCommitSHA != "" {
			fmt.Fprintf(w, "             base %s\n", sug.BaseCommitSHA)
		}
		if sug.Description != "" {
			fmt.Fprintf(w, "             %s\n", sug.Description)
		}
	}
	fmt.Fprintln(w, "\nActions:")
	fmt.Fprintln(w, "  jul show <id>      Show diff")
	fmt.Fprintln(w, "  jul apply <id>     Apply to draft")
	fmt.Fprintln(w, "  jul reject <id>    Reject")
}

func RenderSuggestionCreated(w io.Writer, sug client.Suggestion) {
	fmt.Fprintf(w, "suggestion %s created\n", sug.SuggestionID)
}

func RenderSuggestionUpdated(w io.Writer, action string, sug client.Suggestion) {
	fmt.Fprintf(w, "%s %s\n", action, sug.SuggestionID)
}

func formatConfidence(value float64) string {
	if value <= 0 {
		return "(?)"
	}
	return fmt.Sprintf("(%.0f%%)", value*100)
}
