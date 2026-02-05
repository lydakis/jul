package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/lydakis/jul/cli/internal/metrics"
)

func printTimings(label string, timings metrics.Timings) {
	if timings.TotalMs <= 0 && len(timings.PhaseMs) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "%s timings (ms): total=%d\n", label, timings.TotalMs)
	if len(timings.PhaseMs) == 0 {
		return
	}
	keys := make([]string, 0, len(timings.PhaseMs))
	for key := range timings.PhaseMs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(os.Stderr, "  %s=%d\n", key, timings.PhaseMs[key])
	}
}
