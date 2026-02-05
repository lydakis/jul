package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lydakis/jul/cli/internal/agent"
	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

type reviewMode string

const (
	reviewModeSummary reviewMode = "summary"
	reviewModeSuggest reviewMode = "suggest"
)

func newReviewCommand() Command {
	return Command{
		Name:    "review",
		Summary: "Run the internal review agent",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("review")
			suggest := fs.Bool("suggest", false, "Create suggestions instead of summary")
			from := fs.String("from", "", "Reuse prior review summary (requires --suggest)")
			_ = fs.Parse(args)

			mode := reviewModeSummary
			if *suggest {
				mode = reviewModeSuggest
			}
			fromID := strings.TrimSpace(*from)
			if fromID != "" && mode != reviewModeSuggest {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "review_invalid_args", "--from requires --suggest", nil)
				} else {
					fmt.Fprintln(os.Stderr, "review failed: --from requires --suggest")
				}
				return 1
			}

			if reviewInternalEnv() {
				return runReviewInternalCommand(mode, fromID, *jsonOut)
			}

			run, err := startBackgroundReview(mode, fromID)
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "review_failed", fmt.Sprintf("review failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
				}
				return 1
			}

			stream := watchStream(*jsonOut, os.Stdout, os.Stderr)
			watch := stream != nil
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			detached := make(chan struct{})
			var sigCh chan os.Signal
			if watch {
				sigCh = make(chan os.Signal, 1)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
				go func() {
					<-sigCh
					close(detached)
					cancel()
				}()
				go func() {
					_ = tailFile(ctx, run.LogPath, stream, "review: ")
				}()
			}

			result, err := waitForReviewResult(ctx, run.ResultPath)
			cancel()
			if watch && sigCh != nil {
				signal.Stop(sigCh)
			}
			if isDetached(detached) {
				return 0
			}
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "review_failed", fmt.Sprintf("review failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
				}
				return 1
			}
			if strings.TrimSpace(result.Error) != "" {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "review_failed", result.Error, nil)
				} else {
					fmt.Fprintf(os.Stderr, "review failed: %s\n", result.Error)
				}
				return 1
			}

			return renderReviewResult(result, *jsonOut)
		},
	}
}

func isDetached(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func runReviewInternalCommand(mode reviewMode, fromReviewID string, jsonOut bool) int {
	started := time.Now().UTC()
	result, err := runReviewInternal(mode, fromReviewID, os.Stdout)
	result.Mode = mode
	if result.StartedAt.IsZero() {
		result.StartedAt = started
	}
	result.FinishedAt = time.Now().UTC()
	if err != nil {
		result.Error = err.Error()
	}
	if path := reviewResultPathEnv(); path != "" {
		_ = writeReviewResult(path, result)
		if err != nil {
			return 1
		}
		return 0
	}
	if err != nil {
		if jsonOut {
			_ = output.EncodeError(os.Stdout, "review_failed", fmt.Sprintf("review failed: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "review failed: %v\n", err)
		}
		return 1
	}
	return renderReviewResult(result, jsonOut)
}

func runReviewInternal(mode reviewMode, fromReviewID string, stream io.Writer) (reviewRunResult, error) {
	if mode == "" {
		mode = reviewModeSummary
	}
	if mode != reviewModeSuggest && strings.TrimSpace(fromReviewID) != "" {
		return reviewRunResult{}, fmt.Errorf("--from requires --suggest")
	}
	baseSHA, changeID, err := reviewBase()
	if err != nil {
		return reviewRunResult{}, err
	}

	repoRoot, err := gitutil.RepoTopLevel()
	if err != nil {
		return reviewRunResult{}, err
	}

	worktree, err := agent.EnsureWorktree(repoRoot, baseSHA, agent.WorktreeOptions{})
	if err != nil {
		if errors.Is(err, agent.ErrMergeInProgress) {
			return reviewRunResult{}, fmt.Errorf("merge in progress; run 'jul merge' first")
		}
		return reviewRunResult{}, err
	}

	diff := reviewDiff(baseSHA)
	files := reviewFiles(baseSHA)

	ctx := agent.ReviewContext{
		Checkpoint: baseSHA,
		ChangeID:   changeID,
		Diff:       diff,
		Files:      files,
		CIResults:  reviewCIResults(baseSHA),
	}
	if mode == reviewModeSuggest && strings.TrimSpace(fromReviewID) != "" {
		note, err := metadata.GetAgentReviewByID(strings.TrimSpace(fromReviewID))
		if err != nil {
			return reviewRunResult{}, err
		}
		if note == nil || strings.TrimSpace(note.Summary) == "" {
			return reviewRunResult{}, fmt.Errorf("review %s not found", strings.TrimSpace(fromReviewID))
		}
		ctx.PriorSummary = strings.TrimSpace(note.Summary)
	}

	action := "review_summary"
	if mode == reviewModeSuggest {
		action = "review_suggest"
	}
	req := agent.ReviewRequest{
		Version:       1,
		Action:        action,
		WorkspacePath: worktree,
		Context:       ctx,
	}

	provider, err := agent.ResolveProvider()
	if err != nil {
		return reviewRunResult{}, err
	}

	resp, err := agent.RunReviewWithStream(context.Background(), provider, req, stream)
	if err != nil {
		return reviewRunResult{}, err
	}

	status := strings.TrimSpace(resp.Status)
	if status == "" {
		status = "completed"
	}
	result := reviewRunResult{
		Mode:     mode,
		Status:   status,
		BaseSHA:  baseSHA,
		ChangeID: changeID,
	}
	if mode == reviewModeSummary {
		result.Summary = strings.TrimSpace(resp.Summary)
		note, err := writeReviewNote(baseSHA, changeID, resp)
		if err != nil {
			return result, err
		}
		result.ReviewID = note.ReviewID
		return result, nil
	}

	seeds, err := collectReviewSuggestions(worktree, baseSHA, resp)
	if err != nil {
		return result, err
	}

	created, err := storeReviewSuggestions(baseSHA, changeID, seeds)
	if err != nil {
		return result, err
	}
	result.Suggestions = created
	return result, nil
}

func renderReviewResult(result reviewRunResult, jsonOut bool) int {
	if jsonOut {
		out := output.ReviewOutput{}
		if result.Mode == reviewModeSummary {
			out.Review = &output.ReviewSummary{
				ReviewID:  strings.TrimSpace(result.ReviewID),
				Status:    strings.TrimSpace(result.Status),
				BaseSHA:   strings.TrimSpace(result.BaseSHA),
				ChangeID:  strings.TrimSpace(result.ChangeID),
				Summary:   strings.TrimSpace(result.Summary),
				Timestamp: result.FinishedAt.UTC().Format(time.RFC3339),
			}
		} else {
			out.Suggestions = result.Suggestions
			if len(result.Suggestions) > 0 {
				out.NextActions = buildSuggestionActions(result.Suggestions)
			}
		}
		return writeJSON(out)
	}

	if result.Mode == reviewModeSummary {
		summary := output.ReviewSummary{
			ReviewID:  strings.TrimSpace(result.ReviewID),
			Status:    strings.TrimSpace(result.Status),
			BaseSHA:   strings.TrimSpace(result.BaseSHA),
			ChangeID:  strings.TrimSpace(result.ChangeID),
			Summary:   strings.TrimSpace(result.Summary),
			Timestamp: result.FinishedAt.UTC().Format(time.RFC3339),
		}
		output.RenderReview(os.Stdout, summary)
		return 0
	}

	if len(result.Suggestions) == 0 {
		fmt.Fprintln(os.Stdout, "No suggestions created.")
		return 0
	}
	fmt.Fprintf(os.Stdout, "%d suggestion(s) created.\n\n", len(result.Suggestions))
	fmt.Fprintln(os.Stdout, "Run 'jul suggestions' to see details.")
	return 0
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

func writeReviewNote(baseSHA, changeID string, resp agent.ReviewResponse) (metadata.AgentReviewNote, error) {
	payload, err := json.Marshal(resp)
	if err != nil {
		return metadata.AgentReviewNote{}, err
	}
	status := strings.TrimSpace(resp.Status)
	if status == "" {
		status = "completed"
	}
	return metadata.WriteAgentReview(metadata.AgentReviewNote{
		BaseCommitSHA: baseSHA,
		ChangeID:      changeID,
		Status:        status,
		Summary:       strings.TrimSpace(resp.Summary),
		Response:      payload,
	})
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
