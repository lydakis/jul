package cli

import (
	"encoding/json"
	"errors"
	"flag"
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
			fs := flag.NewFlagSet("checkpoint", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
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
					fmt.Fprintf(os.Stderr, "failed to record trace: %v\n", err)
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
				fmt.Fprintf(os.Stderr, "checkpoint failed: %v\n", err)
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
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
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
