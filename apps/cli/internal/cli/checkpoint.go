package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/config"
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
			_ = fs.Parse(args)

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

			var res syncer.CheckpointResult
			var err error
			if *adopt {
				if *message != "" && !hookMode {
					fmt.Fprintln(os.Stderr, "warning: --message ignored when adopting HEAD")
				}
				res, err = syncer.AdoptCheckpoint()
			} else {
				res, err = syncer.Checkpoint(*message)
			}
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "checkpoint_failed", fmt.Sprintf("checkpoint failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "checkpoint failed: %v\n", err)
				}
				return 1
			}

			ciExit := 0
			if config.CIRunOnCheckpoint() && !skipCI {
				out := io.Writer(os.Stdout)
				errOut := io.Writer(os.Stderr)
				if *jsonOut {
					out = io.Discard
					errOut = io.Discard
				}
				ciExit = runCIRunWithStream([]string{}, nil, out, errOut, res.CheckpointSHA, "checkpoint")
			}

			if config.ReviewEnabled() && config.ReviewRunOnCheckpoint() && !skipReview {
				if _, _, err := runReview(); err != nil {
					if !errors.Is(err, agent.ErrAgentNotConfigured) && !errors.Is(err, agent.ErrBundledMissing) {
						fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
					}
				}
			}

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
			if ciExit != 0 {
				return ciExit
			}
			return 0
		},
	}
}
