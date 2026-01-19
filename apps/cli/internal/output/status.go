package output

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
)

type StatusPayload struct {
	WorkspaceID string `json:"workspace_id"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	CommitSHA   string `json:"commit_sha"`
	ChangeID    string `json:"change_id"`
	SyncStatus  string `json:"sync_status"`
	Attestation string `json:"attestation"`
	CheckedAt   string `json:"checked_at"`
}

func RenderStatus(w io.Writer, payload StatusPayload, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	fmt.Fprintf(w, "Workspace: %s\n", payload.WorkspaceID)
	fmt.Fprintf(w, "Repo:      %s\n", payload.Repo)
	fmt.Fprintf(w, "Branch:    %s\n", payload.Branch)
	fmt.Fprintf(w, "Commit:    %s\n", payload.CommitSHA)
	if payload.ChangeID != "" {
		fmt.Fprintf(w, "Change:    %s\n", payload.ChangeID)
	}
	fmt.Fprintf(w, "Sync:      %s\n", payload.SyncStatus)
	fmt.Fprintf(w, "Check:     %s\n", payload.Attestation)
	fmt.Fprintf(w, "Checked:   %s\n", payload.CheckedAt)
	return nil
}

func BuildStatusPayload(wsID, repo, branch, commitSHA, changeID string, ws client.Workspace, att *client.Attestation) StatusPayload {
	if repo == "" {
		repo = ws.Repo
	}
	if branch == "" {
		branch = ws.Branch
	}

	status := StatusPayload{
		WorkspaceID: wsID,
		Repo:        repo,
		Branch:      branch,
		CommitSHA:   commitSHA,
		ChangeID:    changeID,
		SyncStatus:  "not_synced",
		Attestation: "unknown",
		CheckedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	if ws.WorkspaceID != "" {
		status.SyncStatus = "synced"
		if status.ChangeID == "" {
			status.ChangeID = ws.LastChangeID
		}
	}
	if att != nil {
		status.Attestation = att.Status
	}
	return status
}
