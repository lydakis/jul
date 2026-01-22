package output

import (
	"fmt"
	"io"
)

type ReflogEntry struct {
	CommitSHA string `json:"commit_sha"`
	Kind      string `json:"kind"`
	Message   string `json:"message,omitempty"`
	When      string `json:"when,omitempty"`
}

func RenderReflog(w io.Writer, entries []ReflogEntry) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No reflog entries.")
		return
	}
	for _, entry := range entries {
		if entry.Kind == "draft" {
			fmt.Fprintf(w, "        └─ draft sync (%s)\n", entry.When)
			continue
		}
		msg := entry.Message
		if msg == "" {
			msg = "checkpoint"
		}
		fmt.Fprintf(w, "%s checkpoint \"%s\" (%s)\n", entry.CommitSHA, msg, entry.When)
	}
}
