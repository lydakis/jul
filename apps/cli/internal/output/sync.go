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
		if res.RemoteProblem != "" {
			fmt.Fprintf(w, "  (%s)\n", res.RemoteProblem)
		} else {
			fmt.Fprintln(w, "  (No remote configured)")
		}
	} else {
		fmt.Fprintf(w, "  %sSync ref pushed (%s)\n", ok, res.SyncRef)
	}
	if res.FastForwarded {
		fmt.Fprintf(w, "  %sBase fast-forwarded (clean)\n", ok)
	}
	if res.BaseAdvanced {
		fmt.Fprintf(w, "  %sBase advanced — run 'jul ws restack' when ready\n", warn)
	}
	if res.Diverged {
		fmt.Fprintf(w, "  %sWorkspace lease mismatch — run 'jul ws checkout'\n", warn)
	}
}
