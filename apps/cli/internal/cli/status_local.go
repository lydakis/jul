package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

type localStatus struct {
	WorkspaceID        string            `json:"workspace_id"`
	Repo               string            `json:"repo"`
	Branch             string            `json:"branch"`
	DraftSHA           string            `json:"draft_sha"`
	ChangeID           string            `json:"change_id"`
	SyncStatus         string            `json:"sync_status"`
	LastCheckpoint     *checkpointStatus `json:"last_checkpoint,omitempty"`
	AttestationStatus  string            `json:"attestation_status,omitempty"`
	SuggestionsPending int               `json:"suggestions_pending"`
}

type checkpointStatus struct {
	CommitSHA string `json:"commit_sha"`
	Message   string `json:"message"`
	Author    string `json:"author"`
	When      string `json:"when"`
	ChangeID  string `json:"change_id"`
}

func buildLocalStatus() (localStatus, error) {
	info, err := gitutil.CurrentCommit()
	if err != nil {
		return localStatus{}, err
	}
	repoName := config.RepoName()
	if repoName != "" {
		info.RepoName = repoName
	}

	user, workspace := workspaceParts()
	if workspace == "" {
		workspace = "@"
	}
	wsID := config.WorkspaceID()

	draftSHA := ""
	if ref, err := syncRef(user, workspace); err == nil && gitutil.RefExists(ref) {
		if sha, err := gitutil.ResolveRef(ref); err == nil {
			draftSHA = sha
		}
	}
	if draftSHA == "" {
		ref := workspaceRef(user, workspace)
		if gitutil.RefExists(ref) {
			if sha, err := gitutil.ResolveRef(ref); err == nil {
				draftSHA = sha
			}
		}
	}
	if draftSHA == "" {
		draftSHA = info.SHA
	}

	var checkpoint *checkpointStatus
	var att *client.Attestation
	last, err := latestCheckpoint()
	if err != nil {
		return localStatus{}, err
	}
	if last != nil {
		checkpoint = &checkpointStatus{
			CommitSHA: last.SHA,
			Message:   firstLine(last.Message),
			Author:    last.Author,
			When:      last.When.Format("2006-01-02 15:04:05"),
			ChangeID:  last.ChangeID,
		}
		att, _ = metadata.GetAttestation(last.SHA)
	}
	if att == nil {
		att, _ = metadata.GetAttestation(info.SHA)
	}

	changeID := info.ChangeID
	if checkpoint != nil && checkpoint.ChangeID != "" {
		changeID = checkpoint.ChangeID
	}
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(info.SHA)
	}

	suggestions, err := metadata.ListSuggestions(changeID, "open", 1000)
	if err != nil {
		return localStatus{}, err
	}

	status := localStatus{
		WorkspaceID:        wsID,
		Repo:               info.RepoName,
		Branch:             info.Branch,
		DraftSHA:           draftSHA,
		ChangeID:           changeID,
		SyncStatus:         "local",
		LastCheckpoint:     checkpoint,
		SuggestionsPending: len(suggestions),
	}
	if att != nil {
		status.AttestationStatus = att.Status
	}
	return status, nil
}

func renderLocalStatus(w io.Writer, status localStatus) {
	fmt.Fprintf(w, "Workspace: %s\n", status.WorkspaceID)
	if status.DraftSHA != "" {
		fmt.Fprintf(w, "Draft:     %s\n", status.DraftSHA)
	}
	if status.ChangeID != "" {
		fmt.Fprintf(w, "Change:    %s\n", status.ChangeID)
	}
	if status.LastCheckpoint != nil {
		fmt.Fprintf(w, "Checkpoint: %s \"%s\"\n", status.LastCheckpoint.CommitSHA, status.LastCheckpoint.Message)
	}
	if status.AttestationStatus != "" {
		fmt.Fprintf(w, "CI:        %s\n", strings.ToLower(status.AttestationStatus))
	}
	if status.SuggestionsPending > 0 {
		fmt.Fprintf(w, "Suggestions: %d pending\n", status.SuggestionsPending)
	}
	fmt.Fprintf(w, "Sync:      %s\n", status.SyncStatus)
}
