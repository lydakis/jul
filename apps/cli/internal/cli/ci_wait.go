package cli

import (
	"context"
	"time"

	cicmd "github.com/lydakis/jul/cli/internal/ci"
)

func waitForCIRun(ctx context.Context, runID string) (*cicmd.RunRecord, error) {
	if runID == "" {
		return nil, nil
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		run, err := cicmd.ReadRun(runID)
		if err != nil {
			return nil, err
		}
		if run != nil && run.Status != "" && run.Status != "running" && !run.FinishedAt.IsZero() {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
