package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

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

			output.RenderShow(os.Stdout, payload)
			return 0
		},
	}
}

func buildShowPayload(id string) (output.ShowResult, error) {
	if suggestion, ok, err := metadata.GetSuggestionByID(id); err == nil && ok {
		diffstat := diffStat(suggestion.BaseCommitSHA, suggestion.SuggestedCommitSHA)
		return output.ShowResult{
			Type:       "suggestion",
			Suggestion: &suggestion,
			DiffStat:   diffstat,
		}, nil
	} else if err != nil {
		return output.ShowResult{}, err
	}

	sha, err := gitutil.Git("rev-parse", id)
	if err != nil {
		return output.ShowResult{}, fmt.Errorf("failed to resolve %s", id)
	}
	message, _ := gitutil.CommitMessage(sha)
	author, _ := gitutil.Git("log", "-1", "--format=%an", sha)
	when, _ := gitutil.Git("log", "-1", "--format=%cI", sha)
	changeID := gitutil.ExtractChangeID(message)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(sha)
	}

	attView, _ := resolveAttestationView(sha)
	diffstat := diffStatParent(sha)
	return output.ShowResult{
		Type:                     "checkpoint",
		CommitSHA:                sha,
		ChangeID:                 changeID,
		Message:                  strings.TrimSpace(message),
		Author:                   strings.TrimSpace(author),
		When:                     strings.TrimSpace(when),
		Attestation:              attView.Attestation,
		AttestationStale:         attView.Stale,
		AttestationInheritedFrom: attView.InheritedFrom,
		DiffStat:                 diffstat,
	}, nil
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
