package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
)

func newCheckpointCommand() Command {
	return Command{
		Name:    "checkpoint",
		Summary: "Record a checkpoint for the current commit",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("checkpoint", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			info, err := gitutil.CurrentCommit()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read git state: %v\n", err)
				return 1
			}
			repoName := config.RepoName()
			if repoName != "" {
				info.RepoName = repoName
			}

			payload := client.SyncPayload{
				WorkspaceID: config.WorkspaceID(),
				Repo:        info.RepoName,
				Branch:      info.Branch,
				CommitSHA:   info.SHA,
				ChangeID:    info.ChangeID,
				Message:     info.Message,
				Author:      info.Author,
				CommittedAt: info.Committed,
			}

			cli := client.New(config.BaseURL())
			res, err := cli.Checkpoint(payload)
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

			fmt.Fprintf(os.Stdout, "checkpoint %s (%s)\n", res.Revision.CommitSHA, res.Change.ChangeID)
			return 0
		},
	}
}
