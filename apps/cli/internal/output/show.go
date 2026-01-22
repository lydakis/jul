package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
)

type ShowResult struct {
	Type        string              `json:"type"`
	CommitSHA   string              `json:"commit_sha,omitempty"`
	ChangeID    string              `json:"change_id,omitempty"`
	Message     string              `json:"message,omitempty"`
	Author      string              `json:"author,omitempty"`
	When        string              `json:"when,omitempty"`
	Attestation *client.Attestation `json:"attestation,omitempty"`
	Suggestion  *client.Suggestion  `json:"suggestion,omitempty"`
	DiffStat    string              `json:"diffstat,omitempty"`
}

func RenderShow(w io.Writer, payload ShowResult) {
	if payload.Type == "suggestion" && payload.Suggestion != nil {
		fmt.Fprintf(w, "Suggestion: %s\n", payload.Suggestion.SuggestionID)
		fmt.Fprintf(w, "Change-Id: %s\n", payload.Suggestion.ChangeID)
		fmt.Fprintf(w, "Status: %s\n", payload.Suggestion.Status)
		fmt.Fprintf(w, "Reason: %s\n", payload.Suggestion.Reason)
		if payload.Suggestion.Description != "" {
			fmt.Fprintf(w, "Description: %s\n", payload.Suggestion.Description)
		}
		fmt.Fprintf(w, "Base: %s\n", payload.Suggestion.BaseCommitSHA)
		fmt.Fprintf(w, "Suggested: %s\n", payload.Suggestion.SuggestedCommitSHA)
		if payload.DiffStat != "" {
			fmt.Fprintln(w, "\nFiles changed:")
			fmt.Fprintln(w, payload.DiffStat)
		}
		return
	}

	fmt.Fprintf(w, "Checkpoint: %s\n", payload.CommitSHA)
	if payload.Message != "" {
		msg := strings.Split(payload.Message, "\n")[0]
		fmt.Fprintf(w, "Message: %q\n", msg)
	}
	if payload.Author != "" {
		fmt.Fprintf(w, "Author: %s\n", payload.Author)
	}
	if payload.When != "" {
		fmt.Fprintf(w, "Date: %s\n", payload.When)
	}
	if payload.ChangeID != "" {
		fmt.Fprintf(w, "Change-Id: %s\n", payload.ChangeID)
	}
	if payload.Attestation != nil {
		fmt.Fprintf(w, "\nAttestation: %s\n", payload.Attestation.Status)
	}
	if payload.DiffStat != "" {
		fmt.Fprintln(w, "\nFiles changed:")
		fmt.Fprintln(w, payload.DiffStat)
	}
}
