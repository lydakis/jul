package output

import (
	"fmt"
	"io"
)

type LogEntry struct {
	CommitSHA         string `json:"commit_sha"`
	ChangeID          string `json:"change_id"`
	Author            string `json:"author"`
	Message           string `json:"message"`
	When              string `json:"when"`
	AttestationStatus string `json:"attestation_status,omitempty"`
	Suggestions       int    `json:"suggestions,omitempty"`
}

func RenderLog(w io.Writer, entries []LogEntry, opts Options) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No checkpoints.")
		return
	}
	for _, entry := range entries {
		line := fmt.Sprintf("%s (%s) %q", entry.CommitSHA, entry.When, entry.Message)
		fmt.Fprintln(w, line)
		if entry.Author != "" {
			fmt.Fprintf(w, "        Author: %s\n", entry.Author)
		}
		if entry.AttestationStatus != "" {
			icon := statusIconColored(entry.AttestationStatus, opts)
			fmt.Fprintf(w, "        %sCI %s\n", icon, statusText(entry.AttestationStatus, opts))
		}
		if entry.Suggestions > 0 {
			fmt.Fprintf(w, "        %d suggestion(s) pending\n", entry.Suggestions)
		}
		fmt.Fprintln(w, "")
	}
}
