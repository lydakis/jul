package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
)

type ReviewOutput struct {
	Review      *ReviewSummary      `json:"review,omitempty"`
	Suggestions []client.Suggestion `json:"suggestions,omitempty"`
	NextActions []NextAction        `json:"next_actions,omitempty"`
}

type ReviewSummary struct {
	ReviewID  string `json:"review_id,omitempty"`
	Status    string `json:"status,omitempty"`
	BaseSHA   string `json:"base_sha,omitempty"`
	ChangeID  string `json:"change_id,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type NextAction struct {
	Action  string `json:"action"`
	Command string `json:"command"`
}

func RenderReview(w io.Writer, summary ReviewSummary) {
	if summary.BaseSHA != "" {
		fmt.Fprintf(w, "Review for %s\n", summary.BaseSHA)
	} else {
		fmt.Fprintln(w, "Review summary")
	}
	if strings.TrimSpace(summary.Summary) == "" {
		fmt.Fprintln(w, "No summary returned.")
	} else {
		fmt.Fprintln(w, strings.TrimSpace(summary.Summary))
	}
	if summary.ReviewID != "" {
		fmt.Fprintf(w, "\nReview ID: %s\n", summary.ReviewID)
	}
}
