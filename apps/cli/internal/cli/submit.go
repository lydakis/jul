package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

func newSubmitCommand() Command {
	return Command{
		Name:    "submit",
		Summary: "Create or update the review for this workspace",
		Run: func(args []string) int {
			return runSubmit(args)
		},
	}
}

func runSubmit(args []string) int {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	jsonOut := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)

	state, err := submitReview()
	if err != nil {
		fmt.Fprintf(os.Stderr, "submit failed: %v\n", err)
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(state); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
			return 1
		}
		return 0
	}

	_, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	fmt.Fprintf(os.Stdout, "Review updated for workspace '%s'\n", workspace)
	fmt.Fprintf(os.Stdout, "  Change-Id: %s\n", state.ChangeID)
	fmt.Fprintf(os.Stdout, "  Checkpoint: %s\n", state.LatestCheckpoint)
	return 0
}

func submitReview() (metadata.ReviewState, error) {
	draftSHA, err := currentDraftSHA()
	if err != nil {
		return metadata.ReviewState{}, err
	}
	draftMsg, _ := gitutil.CommitMessage(draftSHA)
	changeID := gitutil.ExtractChangeID(draftMsg)
	if changeID == "" {
		changeID = changeIDForCommit(draftSHA)
	}

	checkpoint, err := latestCheckpointForChange(changeID)
	if err != nil {
		return metadata.ReviewState{}, err
	}
	if checkpoint == nil {
		return metadata.ReviewState{}, fmt.Errorf("checkpoint required before submit")
	}
	anchorSHA, checkpoints, err := changeMetaFromCheckpoints(changeID)
	if err != nil {
		return metadata.ReviewState{}, err
	}
	if strings.TrimSpace(anchorSHA) == "" {
		anchorSHA = checkpoint.SHA
	}

	meta, ok, err := metadata.ReadChangeMeta(anchorSHA)
	if err != nil {
		return metadata.ReviewState{}, err
	}
	if !ok {
		meta = metadata.ChangeMeta{}
	}
	if meta.ChangeID == "" {
		meta.ChangeID = changeID
	}
	if meta.AnchorSHA == "" {
		meta.AnchorSHA = anchorSHA
	}
	if len(checkpoints) > 0 {
		meta.Checkpoints = checkpoints
	}
	if err := metadata.WriteChangeMeta(meta); err != nil {
		return metadata.ReviewState{}, err
	}

	user, workspace := workspaceParts()
	workspaceID := strings.TrimSpace(user)
	if workspace != "" {
		if workspaceID != "" {
			workspaceID += "/"
		}
		workspaceID += workspace
	}
	state := metadata.ReviewState{
		ChangeID:        changeID,
		AnchorSHA:       anchorSHA,
		LatestCheckpoint: checkpoint.SHA,
		Status:          "open",
		WorkspaceID:     workspaceID,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := metadata.WriteReviewState(state); err != nil {
		return metadata.ReviewState{}, err
	}
	return state, nil
}
