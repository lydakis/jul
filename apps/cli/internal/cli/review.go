package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

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
				out := output.ReviewOutput{
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

			output.RenderReview(os.Stdout, summary)
			return 0
		},
	}
}

func runReview() ([]client.Suggestion, output.ReviewSummary, error) {
	baseSHA, changeID, err := reviewBase()
	if err != nil {
		return nil, output.ReviewSummary{}, err
	}

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return nil, output.ReviewSummary{}, err
	}

	worktree, err := agent.EnsureWorktree(repoRoot, baseSHA)
	if err != nil {
		return nil, output.ReviewSummary{}, err
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
		return nil, output.ReviewSummary{}, err
	}

	resp, err := agent.RunReview(context.Background(), provider, req)
	if err != nil {
		return nil, output.ReviewSummary{}, err
	}

	seeds, err := collectReviewSuggestions(worktree, baseSHA, resp)
	if err != nil {
		return nil, output.ReviewSummary{}, err
	}

	created, err := storeReviewSuggestions(baseSHA, changeID, seeds)
	if err != nil {
		return nil, output.ReviewSummary{}, err
	}

	if err := writeReviewNote(baseSHA, changeID, resp); err != nil {
		return nil, output.ReviewSummary{}, err
	}

	summary := output.ReviewSummary{
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
		out, err := gitutil.Git("show", "--format=", "--root", baseSHA)
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
	out, err := gitutil.Git("diff-tree", "--no-commit-id", "--name-only", "-r", "--root", baseSHA)
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

func writeReviewNote(baseSHA, changeID string, resp agent.ReviewResponse) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	status := strings.TrimSpace(resp.Status)
	if status == "" {
		status = "completed"
	}
	_, err = metadata.WriteReview(metadata.ReviewNote{
		BaseCommitSHA: baseSHA,
		ChangeID:      changeID,
		Status:        status,
		Response:      payload,
	})
	return err
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

func collectReviewSuggestions(worktree, baseSHA string, resp agent.ReviewResponse) ([]agent.ReviewSuggestion, error) {
	seeds := make(map[string]agent.ReviewSuggestion)
	for _, sug := range resp.Suggestions {
		commit := strings.TrimSpace(sug.Commit)
		if commit == "" {
			continue
		}
		seeds[commit] = sug
	}

	if len(seeds) == 0 {
		commit, err := autoCommitWorktree(worktree, "agent: review")
		if err != nil {
			return nil, err
		}
		if commit != "" {
			seeds[commit] = agent.ReviewSuggestion{
				Commit:      commit,
				Reason:      "agent_review",
				Description: "agent generated changes",
				Confidence:  0,
			}
		}
	}

	commits, err := worktreeCommits(worktree, baseSHA)
	if err != nil {
		return nil, err
	}
	for _, commit := range commits {
		if _, ok := seeds[commit]; ok {
			continue
		}
		seeds[commit] = agent.ReviewSuggestion{
			Commit:      commit,
			Reason:      "agent_review",
			Description: "agent generated changes",
		}
	}

	out := make([]agent.ReviewSuggestion, 0, len(seeds))
	for _, sug := range seeds {
		out = append(out, sug)
	}
	return out, nil
}

func autoCommitWorktree(worktree, message string) (string, error) {
	dirty, err := worktreeDirty(worktree)
	if err != nil || !dirty {
		return "", err
	}
	// Avoid staging agent context artifacts if they somehow end up in the worktree.
	if err := gitDir(worktree, nil, "add", "-A", "--", ".", ":(exclude)jul-review-*.txt"); err != nil {
		return "", err
	}
	env := map[string]string{
		"GIT_AUTHOR_NAME":     "Jul Agent",
		"GIT_AUTHOR_EMAIL":    "agent@jul.local",
		"GIT_COMMITTER_NAME":  "Jul Agent",
		"GIT_COMMITTER_EMAIL": "agent@jul.local",
	}
	if err := gitDir(worktree, env, "commit", "-m", message, "--no-gpg-sign"); err != nil {
		return "", err
	}
	return gitOutputDir(worktree, "rev-parse", "HEAD")
}

func worktreeDirty(worktree string) (bool, error) {
	out, err := gitOutputDir(worktree, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func worktreeCommits(worktree, baseSHA string) ([]string, error) {
	if strings.TrimSpace(baseSHA) == "" {
		return nil, nil
	}
	out, err := gitOutputDir(worktree, "rev-list", "--reverse", fmt.Sprintf("%s..HEAD", baseSHA))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	commits := make([]string, 0, len(lines))
	for _, line := range lines {
		sha := strings.TrimSpace(line)
		if sha == "" {
			continue
		}
		commits = append(commits, sha)
	}
	return commits, nil
}

func gitOutputDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func gitDir(dir string, env map[string]string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(env)...)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return nil
}

func flattenEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
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

func buildSuggestionActions(suggestions []client.Suggestion) []output.NextAction {
	actions := make([]output.NextAction, 0, len(suggestions))
	for _, sug := range suggestions {
		if sug.SuggestionID == "" {
			continue
		}
		actions = append(actions, output.NextAction{
			Action:  "apply",
			Command: fmt.Sprintf("jul apply %s --json", sug.SuggestionID),
		})
		actions = append(actions, output.NextAction{
			Action:  "reject",
			Command: fmt.Sprintf("jul reject %s --json", sug.SuggestionID),
		})
		actions = append(actions, output.NextAction{
			Action:  "show",
			Command: fmt.Sprintf("jul show %s --json", sug.SuggestionID),
		})
	}
	return actions
}
