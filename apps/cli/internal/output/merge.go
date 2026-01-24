package output

import (
	"fmt"
	"io"
)

type MergeOutput struct {
	Merge       MergeSummary `json:"merge"`
	NextActions []NextAction `json:"next_actions,omitempty"`
}

type MergeSummary struct {
	Status       string   `json:"status"`
	SuggestionID string   `json:"suggestion_id,omitempty"`
	Applied      bool     `json:"applied,omitempty"`
	Conflicts    []string `json:"conflicts,omitempty"`
}

func RenderMerge(w io.Writer, summary MergeSummary) {
	opts := DefaultOptions()
	ok := statusIconColored("pass", opts)
	if ok == "" {
		ok = statusIcon("pass", opts)
	}
	warn := statusIconColored("warning", opts)
	if warn == "" {
		warn = statusIcon("warning", opts)
	}

	switch summary.Status {
	case "up_to_date":
		fmt.Fprintf(w, "%s No merge needed.\n", ok)
		return
	case "resolved":
		fmt.Fprintln(w, "Agent resolving conflicts...")
		if summary.SuggestionID != "" {
			fmt.Fprintf(w, "\nResolution ready as suggestion [%s].\n", summary.SuggestionID)
		}
		if summary.Applied {
			fmt.Fprintf(w, "  %sMerged\n", ok)
		}
		return
	default:
		if len(summary.Conflicts) > 0 {
			fmt.Fprintf(w, "%s Conflicts detected:\n", warn)
			for _, file := range summary.Conflicts {
				fmt.Fprintf(w, "  - %s\n", file)
			}
		}
	}
}
