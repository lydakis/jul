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
		if res.RemotePushed {
			fmt.Fprintf(w, "  %sSync ref pushed (%s)\n", ok, res.SyncRef)
		} else {
			fmt.Fprintf(w, "  %sSync ref not pushed (%s)\n", warn, res.SyncRef)
		}
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
	if res.RemoteProblem != "" && res.RemoteName != "" && !res.BaseAdvanced && !res.Diverged {
		fmt.Fprintf(w, "  %s%s\n", warn, res.RemoteProblem)
	}
	for _, warning := range res.Warnings {
		if warning == "" {
			continue
		}
		fmt.Fprintf(w, "  %s%s\n", warn, warning)
	}
}
