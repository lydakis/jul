package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

type showPayload struct {
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

func newShowCommand() Command {
	return Command{
		Name:    "show",
		Summary: "Show details of a checkpoint or suggestion",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("show", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			id := strings.TrimSpace(fs.Arg(0))
			if id == "" {
				fmt.Fprintln(os.Stderr, "id required")
				return 1
			}

			payload, err := buildShowPayload(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "show failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(payload); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			renderShowPayload(os.Stdout, payload)
			return 0
		},
	}
}

func buildShowPayload(id string) (showPayload, error) {
	if suggestion, ok, err := metadata.GetSuggestionByID(id); err == nil && ok {
		diffstat := diffStat(suggestion.BaseCommitSHA, suggestion.SuggestedCommitSHA)
		return showPayload{
			Type:       "suggestion",
			Suggestion: &suggestion,
			DiffStat:   diffstat,
		}, nil
	} else if err != nil {
		return showPayload{}, err
	}

	sha, err := gitutil.Git("rev-parse", id)
	if err != nil {
		return showPayload{}, fmt.Errorf("failed to resolve %s", id)
	}
	message, _ := gitutil.CommitMessage(sha)
	author, _ := gitutil.Git("log", "-1", "--format=%an", sha)
	when, _ := gitutil.Git("log", "-1", "--format=%cI", sha)
	changeID := gitutil.ExtractChangeID(message)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(sha)
	}

	att, _ := metadata.GetAttestation(sha)
	diffstat := diffStatParent(sha)
	return showPayload{
		Type:        "checkpoint",
		CommitSHA:   sha,
		ChangeID:    changeID,
		Message:     strings.TrimSpace(message),
		Author:      strings.TrimSpace(author),
		When:        strings.TrimSpace(when),
		Attestation: att,
		DiffStat:    diffstat,
	}, nil
}

func renderShowPayload(w io.Writer, payload showPayload) {
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
		fmt.Fprintf(w, "Message: %q\n", firstLine(payload.Message))
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

func diffStat(from, to string) string {
	if strings.TrimSpace(to) == "" {
		return ""
	}
	args := []string{"diff", "--stat"}
	if strings.TrimSpace(from) != "" {
		args = append(args, from, to)
	} else {
		args = append(args, to)
	}
	out, err := gitutil.Git(args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func diffStatParent(sha string) string {
	if strings.TrimSpace(sha) == "" {
		return ""
	}
	parent, err := gitutil.Git("rev-parse", sha+"^")
	if err != nil {
		out, err := gitutil.Git("diff", "--stat", "--root", sha)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(out)
	}
	return diffStat(parent, sha)
}
