package output

import (
	"fmt"
	"io"

	"github.com/lydakis/jul/cli/internal/syncer"
)

func RenderSync(w io.Writer, res syncer.Result, opts Options) {
	fmt.Fprintln(w, "Syncing...")
	ok := statusIconColored("pass", opts)
	if ok == "" {
		ok = statusIcon("pass", opts)
	}
	warn := statusIconColored("warning", opts)
	if warn == "" {
		warn = statusIcon("warning", opts)
	}
	fmt.Fprintf(w, "  %sDraft committed (%s)\n", ok, res.DraftSHA)
	if res.RemoteName == "" {
		fmt.Fprintf(w, "  %sWorkspace ref updated (local)\n", ok)
		if res.RemoteProblem != "" {
			fmt.Fprintf(w, "  (%s)\n", res.RemoteProblem)
		} else {
			fmt.Fprintln(w, "  (No remote configured)")
		}
		return
	}
	fmt.Fprintf(w, "  %sSync ref pushed (%s)\n", ok, res.SyncRef)
	if res.Diverged {
		fmt.Fprintf(w, "  %sWorkspace diverged — run 'jul merge' when ready\n", warn)
		return
	}
	if res.AutoMerged {
		fmt.Fprintf(w, "  %sAuto-merged (no conflicts)\n", ok)
	}
	if res.WorkspaceUpdated {
		fmt.Fprintf(w, "  %sWorkspace ref updated\n", ok)
	}
}
