package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

func newSubmitCommand() Command {
	return Command{
		Name:    "submit",
		Summary: "Create or update the change request for this workspace",
		Run: func(args []string) int {
			return runSubmit(args)
		},
	}
}

func runSubmit(args []string) int {
	fs, jsonOut := newFlagSet("submit")
	_ = fs.Parse(args)

	state, err := submitReview()
	if err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "submit_failed", fmt.Sprintf("submit failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "submit failed: %v\n", err)
		}
		return 1
	}

	if *jsonOut {
		return writeJSON(state)
	}

	renderSubmitOutput(state)
	return 0
}

func renderSubmitOutput(state metadata.ChangeRequestState) {
	if strings.TrimSpace(state.WorkspaceID) != "" {
		fmt.Fprintf(os.Stdout, "CR updated for workspace '%s'\n", state.WorkspaceID)
	} else {
		fmt.Fprintln(os.Stdout, "CR updated.")
	}
	if state.ChangeID != "" {
		fmt.Fprintf(os.Stdout, "  Change-Id: %s\n", state.ChangeID)
	}
	if state.LatestCheckpoint != "" {
		fmt.Fprintf(os.Stdout, "  Checkpoint: %s\n", state.LatestCheckpoint)
	}
}

func submitReview() (metadata.ChangeRequestState, error) {
	draftSHA, err := currentDraftSHA()
	if err != nil {
		return metadata.ChangeRequestState{}, err
	}
	draftMsg, _ := gitutil.CommitMessage(draftSHA)
	changeID := gitutil.ExtractChangeID(draftMsg)
	if changeID == "" {
		changeID = changeIDForCommit(draftSHA)
	}

	checkpoint, err := latestCheckpointForChange(changeID)
	if err != nil {
		return metadata.ChangeRequestState{}, err
	}
	if checkpoint == nil {
		return metadata.ChangeRequestState{}, fmt.Errorf("checkpoint required before submit")
	}
	anchorSHA, checkpoints, err := changeMetaFromCheckpoints(changeID)
	if err != nil {
		return metadata.ChangeRequestState{}, err
	}
	if strings.TrimSpace(anchorSHA) == "" {
		anchorSHA = checkpoint.SHA
	}

	meta, ok, err := metadata.ReadChangeMeta(anchorSHA)
	if err != nil {
		return metadata.ChangeRequestState{}, err
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
		return metadata.ChangeRequestState{}, err
	}

	user, workspace := workspaceParts()
	workspaceID := strings.TrimSpace(user)
	if workspace != "" {
		if workspaceID != "" {
			workspaceID += "/"
		}
		workspaceID += workspace
	}
	state := metadata.ChangeRequestState{
		ChangeID:         changeID,
		AnchorSHA:        anchorSHA,
		LatestCheckpoint: checkpoint.SHA,
		Status:           "open",
		WorkspaceID:      workspaceID,
		UpdatedAt:        time.Now().UTC(),
	}
	if err := metadata.WriteChangeRequestState(state); err != nil {
		return metadata.ChangeRequestState{}, err
	}
	return state, nil
}
