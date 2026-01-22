package output

import (
	"fmt"
	"io"

	"github.com/lydakis/jul/cli/internal/client"
)

type ReviewOutput struct {
	Review      ReviewSummary       `json:"review"`
	Suggestions []client.Suggestion `json:"suggestions,omitempty"`
	NextActions []NextAction        `json:"next_actions,omitempty"`
}

type ReviewSummary struct {
	Status    string `json:"status"`
	BaseSHA   string `json:"base_sha,omitempty"`
	ChangeID  string `json:"change_id,omitempty"`
	Created   int    `json:"suggestions_created"`
	Timestamp string `json:"timestamp"`
}

type NextAction struct {
	Action  string `json:"action"`
	Command string `json:"command"`
}

func RenderReview(w io.Writer, summary ReviewSummary) {
	if summary.BaseSHA != "" {
		fmt.Fprintf(w, "Running review on %s...\n", summary.BaseSHA)
	} else {
		fmt.Fprintln(w, "Running review...")
	}
	if summary.Created == 0 {
		fmt.Fprintln(w, "  ✓ No suggestions created")
		return
	}
	fmt.Fprintf(w, "  ⚠ %d suggestion(s) created\n\n", summary.Created)
	fmt.Fprintln(w, "Run 'jul suggestions' to see details.")
}
