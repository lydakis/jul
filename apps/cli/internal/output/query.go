package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
)

func RenderQuery(w io.Writer, results []client.QueryResult) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No results.")
		return
	}
	for _, res := range results {
		line := firstLine(res.Message)
		status := res.TestStatus
		if status == "" {
			status = res.AttestationStatus
		}
		if status == "" {
			status = "unknown"
		}
		coverage := ""
		if res.CoverageLinePct != nil {
			coverage = fmt.Sprintf(", %.1f%% coverage", *res.CoverageLinePct)
		}
		fmt.Fprintf(w, "%s %s (%s%s)\n", res.CommitSHA, strings.TrimSpace(line), status, coverage)
	}
}

func firstLine(message string) string {
	if message == "" {
		return ""
	}
	lines := strings.Split(message, "\n")
	return lines[0]
}
