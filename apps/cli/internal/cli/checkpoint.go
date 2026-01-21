package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lydakis/jul/cli/internal/config"
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
			noCI := fs.Bool("no-ci", false, "Skip CI run")
			_ = fs.Parse(args)

			res, err := syncer.Checkpoint(*message)
			if err != nil {
				fmt.Fprintf(os.Stderr, "checkpoint failed: %v\n", err)
				return 1
			}

			ciExit := 0
			if config.CIRunOnCheckpoint() && !*noCI {
				out := io.Writer(os.Stdout)
				errOut := io.Writer(os.Stderr)
				if *jsonOut {
					out = io.Discard
					errOut = io.Discard
				}
				ciExit = runCIRunWithStream([]string{}, nil, out, errOut, res.CheckpointSHA)
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

			fmt.Fprintf(os.Stdout, "checkpoint %s (%s)\n", res.CheckpointSHA, res.ChangeID)
			if ciExit != 0 {
				return ciExit
			}
			return 0
		},
	}
}
