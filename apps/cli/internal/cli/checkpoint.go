package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

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
			_ = fs.Parse(args)

			res, err := syncer.Checkpoint(*message)
			if err != nil {
				fmt.Fprintf(os.Stderr, "checkpoint failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			fmt.Fprintf(os.Stdout, "checkpoint %s (%s)\n", res.CheckpointSHA, res.ChangeID)
			return 0
		},
	}
}
