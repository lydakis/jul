package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
)

func RenderChanges(w io.Writer, changes []client.Change, opts Options) {
	if len(changes) == 0 {
		fmt.Fprintln(w, "No changes yet.")
		return
	}
	for _, ch := range changes {
		title := strings.TrimSpace(ch.Title)
		if title == "" {
			title = "untitled"
		}
		fmt.Fprintf(w, "%s %s\n", ch.ChangeID, title)
		if ch.LatestRevision.CommitSHA != "" {
			fmt.Fprintf(w, "        Latest: %s (rev %d)\n", ch.LatestRevision.CommitSHA, ch.LatestRevision.RevIndex)
		}
		if ch.Status != "" {
			fmt.Fprintf(w, "        Status: %s\n", statusText(ch.Status, opts))
		}
		if ch.Author != "" {
			fmt.Fprintf(w, "        Author: %s\n", ch.Author)
		}
		if !ch.CreatedAt.IsZero() {
			fmt.Fprintf(w, "        Created: %s\n", formatTime(ch.CreatedAt))
		}
		if ch.RevisionCount > 0 {
			fmt.Fprintf(w, "        Revisions: %d\n", ch.RevisionCount)
		}
		fmt.Fprintln(w, "")
	}
}

func formatTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}
