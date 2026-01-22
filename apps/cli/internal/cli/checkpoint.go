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
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/notes"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
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
			prompt := fs.String("prompt", "", "Store prompt metadata")
			noCI := fs.Bool("no-ci", false, "Skip CI run")
			noReview := fs.Bool("no-review", false, "Skip review")
			_ = fs.Parse(args)

			res, err := syncer.Checkpoint(*message)
			if err != nil {
				fmt.Fprintf(os.Stderr, "checkpoint failed: %v\n", err)
				return 1
			}

			if strings.TrimSpace(*prompt) != "" {
				note := metadata.PromptNote{
					CommitSHA: res.CheckpointSHA,
					ChangeID:  res.ChangeID,
					Source:    "checkpoint",
					Prompt:    strings.TrimSpace(*prompt),
				}
				if err := metadata.WritePrompt(note); err != nil {
					if errors.Is(err, notes.ErrNoteTooLarge) {
						fmt.Fprintln(os.Stderr, "prompt note too large; skipping")
					} else {
						fmt.Fprintf(os.Stderr, "failed to store prompt note: %v\n", err)
					}
				} else if config.PromptsSyncEnabled() {
					if err := pushPromptNotes(); err != nil {
						fmt.Fprintf(os.Stderr, "failed to push prompt notes: %v\n", err)
					}
				}
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

			if config.ReviewEnabled() && config.ReviewRunOnCheckpoint() && !*noReview {
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

func pushPromptNotes() error {
	remote, err := remotesel.Resolve()
	if err != nil {
		if err == remotesel.ErrNoRemote {
			return nil
		}
		return err
	}
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return err
	}
	ref := notes.RefPrompts
	_, err = gitutil.Git("-C", root, "push", remote.Name, fmt.Sprintf("%s:%s", ref, ref))
	return err
}
