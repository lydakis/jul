package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metrics"
	"github.com/lydakis/jul/cli/internal/output"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func newCheckpointCommand() Command {
	return Command{
		Name:    "checkpoint",
		Summary: "Record a checkpoint for the current commit",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("checkpoint")
			message := fs.String("m", "", "Checkpoint message")
			prompt := fs.String("prompt", "", "Attach prompt metadata via trace")
			adopt := fs.Bool("adopt", false, "Adopt HEAD commit as checkpoint")
			ifConfigured := fs.Bool("if-configured", false, "Only adopt when configured")
			noCI := fs.Bool("no-ci", false, "Skip CI run")
			noReview := fs.Bool("no-review", false, "Skip review")
			debugTimings := fs.Bool("debug-timings", false, "Print timing breakdown to stderr")
			jsonRequested := hasJSONFlag(args)
			if jsonRequested {
				fs.SetOutput(io.Discard)
			}
			if err := fs.Parse(args); err != nil {
				if jsonRequested {
					_ = output.EncodeError(os.Stdout, "checkpoint_invalid_args", err.Error(), nil)
				}
				return 1
			}

			hookMode := os.Getenv("JUL_ADOPT_FROM_HOOK") != ""
			if *adopt && *ifConfigured && !config.CheckpointAdoptOnCommit() {
				if !hookMode {
					fmt.Fprintln(os.Stdout, "checkpoint adopt disabled; enable checkpoint.adopt_on_commit to use --if-configured")
				}
				return 0
			}

			skipCI := *noCI
			skipReview := *noReview
			if hookMode && *adopt {
				if !config.CheckpointAdoptRunCI() {
					skipCI = true
				}
				if !config.CheckpointAdoptRunReview() {
					skipReview = true
				}
			}

			if strings.TrimSpace(*prompt) != "" {
				if _, err := syncer.Trace(syncer.TraceOptions{
					Prompt:          strings.TrimSpace(*prompt),
					Force:           true,
					UpdateCanonical: true,
				}); err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "checkpoint_trace_failed", fmt.Sprintf("failed to record trace: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to record trace: %v\n", err)
					}
					return 1
				}
			}

			stream := watchStream(*jsonOut, os.Stdout, os.Stderr)
			watch := stream != nil

			var res syncer.CheckpointResult
			var err error
			timings := metrics.NewTimings()
			totalStart := time.Now()
			overheadStart := time.Now()
			if *adopt {
				if *message != "" && !hookMode {
					fmt.Fprintln(os.Stderr, "warning: --message ignored when adopting HEAD")
				}
				res, err = syncer.AdoptCheckpoint()
			} else {
				resolvedMessage, resolveErr := resolveCheckpointMessage(*message, stream)
				if resolveErr != nil {
					err = fmt.Errorf("failed to generate checkpoint message: %w", resolveErr)
				} else {
					res, err = syncer.Checkpoint(resolvedMessage)
				}
			}
			timings.Add("overhead", time.Since(overheadStart))
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "checkpoint_failed", fmt.Sprintf("checkpoint failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "checkpoint failed: %v\n", err)
				}
				return 1
			}

			if res.Timings.PhaseMs == nil {
				res.Timings = timings
			} else {
				for k, v := range timings.PhaseMs {
					res.Timings.PhaseMs[k] = v
				}
			}

			if repoRoot, err := gitutil.RepoTopLevel(); err == nil {
				_, _ = updateStatusCacheForCheckpoint(repoRoot, res)
			}

			var ciRun *ciRun
			var reviewRun *reviewRun
			if config.CIRunOnCheckpoint() && !skipCI {
				run, err := startBackgroundCI(res.CheckpointSHA, "checkpoint")
				if err != nil {
					if !*jsonOut {
						fmt.Fprintf(os.Stderr, "failed to start CI: %v\n", err)
					}
				} else {
					ciRun = run
				}
			}

			if config.ReviewEnabled() && config.ReviewRunOnCheckpoint() && !skipReview {
				run, err := startBackgroundReview(reviewModeSuggest, "")
				if err != nil {
					if !*jsonOut && !errors.Is(err, agent.ErrAgentNotConfigured) && !errors.Is(err, agent.ErrBundledMissing) {
						fmt.Fprintf(os.Stderr, "failed to start review: %v\n", err)
					}
				} else {
					reviewRun = run
				}
			}

			ciExit := 0
			if watch && (ciRun != nil || reviewRun != nil) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				detached := make(chan struct{})
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
				go func() {
					<-sigCh
					close(detached)
					cancel()
				}()
				if ciRun != nil {
					go func() {
						_ = tailFile(ctx, ciRun.LogPath, stream, "ci: ")
					}()
				}
				if reviewRun != nil {
					go func() {
						_ = tailFile(ctx, reviewRun.LogPath, stream, "review: ")
					}()
				}

				if ciRun != nil {
					if record, err := waitForCIRun(ctx, ciRun.ID); err == nil && record != nil {
						if !record.StartedAt.IsZero() && !record.FinishedAt.IsZero() {
							res.Timings.Add("ci_runtime", record.FinishedAt.Sub(record.StartedAt))
						}
						ciExit = exitCodeForStatus(record.Status)
					}
				}
				if reviewRun != nil {
					if reviewRes, err := waitForReviewResult(ctx, reviewRun.ResultPath); err == nil {
						start := reviewRes.StartedAt
						end := reviewRes.FinishedAt
						if !start.IsZero() && !end.IsZero() {
							res.Timings.Add("review_runtime", end.Sub(start))
						}
						if strings.TrimSpace(reviewRes.Error) != "" && !*jsonOut {
							fmt.Fprintf(os.Stderr, "review failed: %s\n", strings.TrimSpace(reviewRes.Error))
						}
					}
				}

				cancel()
				signal.Stop(sigCh)
				if isDetached(detached) {
					return 0
				}
			}

			res.Timings.TotalMs = time.Since(totalStart).Milliseconds()

			if *jsonOut {
				if code := writeJSON(res); code != 0 {
					return code
				}
				if ciExit != 0 {
					return ciExit
				}
				return 0
			}

			output.RenderCheckpoint(os.Stdout, res)
			if !watch {
				if ciRun != nil {
					fmt.Fprintln(os.Stdout, "  ⚡ CI running in background... (jul ci status)")
				}
				if reviewRun != nil {
					fmt.Fprintln(os.Stdout, "  ⚡ Review running in background... (jul review --suggest)")
				}
			}
			if *debugTimings {
				printTimings("checkpoint", res.Timings)
			}
			if ciExit != 0 {
				return ciExit
			}
			return 0
		},
	}
}
