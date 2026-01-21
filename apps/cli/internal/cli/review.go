package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

type reviewOutput struct {
	Review      reviewSummary       `json:"review"`
	Suggestions []client.Suggestion `json:"suggestions,omitempty"`
	NextActions []nextAction        `json:"next_actions,omitempty"`
}

type reviewSummary struct {
	Status    string `json:"status"`
	BaseSHA   string `json:"base_sha,omitempty"`
	ChangeID  string `json:"change_id,omitempty"`
	Created   int    `json:"suggestions_created"`
	Timestamp string `json:"timestamp"`
}

type nextAction struct {
	Action  string `json:"action"`
	Command string `json:"command"`
}

func newReviewCommand() Command {
	return Command{
		Name:    "review",
		Summary: "Run the internal review agent",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("review", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			created, summary, err := runReview()
			if err != nil {
				fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				out := reviewOutput{
					Review:      summary,
					Suggestions: created,
				}
				if len(created) > 0 {
					out.NextActions = buildSuggestionActions(created)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			fmt.Fprintf(os.Stdout, "Running review on %s...\n", summary.BaseSHA)
			if summary.Created == 0 {
				fmt.Fprintln(os.Stdout, "  ✓ No suggestions created")
				return 0
			}
			fmt.Fprintf(os.Stdout, "  ⚠ %d suggestion(s) created\n\n", summary.Created)
			fmt.Fprintln(os.Stdout, "Run 'jul suggestions' to see details.")
			return 0
		},
	}
}

func runReview() ([]client.Suggestion, reviewSummary, error) {
	baseSHA, changeID, err := reviewBase()
	if err != nil {
		return nil, reviewSummary{}, err
	}

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return nil, reviewSummary{}, err
	}

	worktree, err := agent.EnsureWorktree(repoRoot, baseSHA)
	if err != nil {
		return nil, reviewSummary{}, err
	}

	diff := reviewDiff(baseSHA)
	files := reviewFiles(baseSHA)

	req := agent.ReviewRequest{
		Version:       1,
		Action:        "review",
		WorkspacePath: worktree,
		Context: agent.ReviewContext{
			Checkpoint: baseSHA,
			ChangeID:   changeID,
			Diff:       diff,
			Files:      files,
			CIResults:  reviewCIResults(baseSHA),
		},
	}

	provider, err := agent.ResolveProvider()
	if err != nil {
		return nil, reviewSummary{}, err
	}

	resp, err := agent.RunReview(context.Background(), provider, req)
	if err != nil {
		return nil, reviewSummary{}, err
	}

	created, err := storeReviewSuggestions(baseSHA, changeID, resp.Suggestions)
	if err != nil {
		return nil, reviewSummary{}, err
	}

	summary := reviewSummary{
		Status:    strings.TrimSpace(resp.Status),
		BaseSHA:   baseSHA,
		ChangeID:  changeID,
		Created:   len(created),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if summary.Status == "" {
		if len(created) == 0 {
			summary.Status = "no_suggestions"
		} else {
			summary.Status = "completed"
		}
	}
	return created, summary, nil
}

func reviewBase() (string, string, error) {
	if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
		return checkpoint.SHA, checkpoint.ChangeID, nil
	}
	draftSHA, err := currentDraftSHA()
	if err != nil {
		return "", "", err
	}
	msg, _ := gitutil.CommitMessage(draftSHA)
	changeID := gitutil.ExtractChangeID(msg)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(draftSHA)
	}
	return draftSHA, changeID, nil
}

func reviewDiff(baseSHA string) string {
	if strings.TrimSpace(baseSHA) == "" {
		return ""
	}
	parent, err := gitutil.ParentOf(baseSHA)
	if err != nil || parent == "" {
		out, err := gitutil.Git("diff", "--root", baseSHA)
		if err != nil {
			return ""
		}
		return out
	}
	out, err := gitutil.Git("diff", parent, baseSHA)
	if err != nil {
		return ""
	}
	return out
}

func reviewFiles(baseSHA string) []agent.ReviewFile {
	if strings.TrimSpace(baseSHA) == "" {
		return nil
	}
	parent, err := gitutil.ParentOf(baseSHA)
	if err != nil || parent == "" {
		parent = ""
	}
	args := []string{"diff", "--name-only"}
	if parent != "" {
		args = append(args, parent, baseSHA)
	} else {
		args = append(args, "--root", baseSHA)
	}
	out, err := gitutil.Git(args...)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	files := make([]agent.ReviewFile, 0, len(lines))
	for _, line := range lines {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		content, _ := reviewFileContent(baseSHA, path)
		files = append(files, agent.ReviewFile{Path: path, Content: content})
	}
	return files
}

func reviewFileContent(baseSHA, path string) (string, error) {
	content, err := gitutil.Git("show", fmt.Sprintf("%s:%s", baseSHA, path))
	if err != nil {
		return "", err
	}
	if len(content) > 64*1024 {
		return content[:64*1024], nil
	}
	return content, nil
}

func reviewCIResults(baseSHA string) json.RawMessage {
	att, err := metadata.GetAttestation(baseSHA)
	if err != nil || att == nil {
		return nil
	}
	if strings.TrimSpace(att.SignalsJSON) == "" {
		return nil
	}
	return json.RawMessage(att.SignalsJSON)
}

func storeReviewSuggestions(baseSHA, changeID string, suggestions []agent.ReviewSuggestion) ([]client.Suggestion, error) {
	minConfidence := config.ReviewMinConfidence()
	created := make([]client.Suggestion, 0, len(suggestions))
	for _, sug := range suggestions {
		if strings.TrimSpace(sug.Commit) == "" {
			continue
		}
		if !passesConfidence(minConfidence, sug.Confidence) {
			continue
		}
		createdSuggestion, err := metadata.CreateSuggestion(metadata.SuggestionCreate{
			ChangeID:           changeID,
			BaseCommitSHA:      baseSHA,
			SuggestedCommitSHA: strings.TrimSpace(sug.Commit),
			CreatedBy:          "agent",
			Reason:             strings.TrimSpace(sug.Reason),
			Description:        strings.TrimSpace(sug.Description),
			Confidence:         sug.Confidence,
		})
		if err != nil {
			return nil, err
		}
		created = append(created, createdSuggestion)
	}
	return created, nil
}

func passesConfidence(min, value float64) bool {
	if min <= 0 {
		return true
	}
	if value <= 1 && min > 1 {
		return value*100 >= min
	}
	if value > 1 && min <= 1 {
		return value >= min*100
	}
	return value >= min
}

func buildSuggestionActions(suggestions []client.Suggestion) []nextAction {
	actions := make([]nextAction, 0, len(suggestions))
	for _, sug := range suggestions {
		if sug.SuggestionID == "" {
			continue
		}
		actions = append(actions, nextAction{
			Action:  "apply",
			Command: fmt.Sprintf("jul apply %s --json", sug.SuggestionID),
		})
		actions = append(actions, nextAction{
			Action:  "reject",
			Command: fmt.Sprintf("jul reject %s --json", sug.SuggestionID),
		})
		actions = append(actions, nextAction{
			Action:  "show",
			Command: fmt.Sprintf("jul show %s --json", sug.SuggestionID),
		})
	}
	return actions
}
