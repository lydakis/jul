package output

import (
	"fmt"
	"io"

	"github.com/lydakis/jul/cli/internal/syncer"
)

func RenderSync(w io.Writer, res syncer.Result) {
	fmt.Fprintln(w, "Syncing...")
	fmt.Fprintf(w, "  ✓ Draft committed (%s)\n", res.DraftSHA)
	if res.RemoteName == "" {
		fmt.Fprintln(w, "  ✓ Workspace ref updated (local)")
		if res.RemoteProblem != "" {
			fmt.Fprintf(w, "  (%s)\n", res.RemoteProblem)
		} else {
			fmt.Fprintln(w, "  (No remote configured)")
		}
		return
	}
	fmt.Fprintf(w, "  ✓ Sync ref pushed (%s)\n", res.SyncRef)
	if res.Diverged {
		fmt.Fprintln(w, "  ⚠ Workspace diverged — run 'jul merge' when ready")
		return
	}
	if res.AutoMerged {
		fmt.Fprintln(w, "  ✓ Auto-merged (no conflicts)")
	}
	if res.WorkspaceUpdated {
		fmt.Fprintln(w, "  ✓ Workspace ref updated")
	}
}
