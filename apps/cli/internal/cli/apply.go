package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func newApplyCommand() Command {
	return Command{
		Name:    "apply",
		Summary: "Apply a suggestion to the current draft",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("apply", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			checkpoint := fs.Bool("checkpoint", false, "Checkpoint after applying")
			force := fs.Bool("force", false, "Apply even if suggestion is stale")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			id := strings.TrimSpace(fs.Arg(0))
			if id == "" {
				fmt.Fprintln(os.Stderr, "suggestion id required")
				return 1
			}

			sug, ok, err := metadata.GetSuggestionByID(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to load suggestion: %v\n", err)
				return 1
			}
			if !ok {
				fmt.Fprintf(os.Stderr, "suggestion %s not found\n", id)
				return 1
			}

			staleBase, err := currentBaseSHA()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to resolve base commit: %v\n", err)
				return 1
			}

			if staleBase != "" && sug.BaseCommitSHA != "" && sug.BaseCommitSHA != staleBase && !*force {
				fmt.Fprintf(os.Stderr, "Suggestion is stale (created for %s, current base is %s)\n", sug.BaseCommitSHA, staleBase)
				fmt.Fprintln(os.Stderr, "Use --force to apply anyway.")
				return 1
			}

			patch, err := suggestionPatch(sug.BaseCommitSHA, sug.SuggestedCommitSHA)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to build patch: %v\n", err)
				return 1
			}
			filesChanged, _ := diffNameOnly(sug.BaseCommitSHA, sug.SuggestedCommitSHA)
			if err := applyPatch(patch, *force); err != nil {
				fmt.Fprintf(os.Stderr, "failed to apply suggestion: %v\n", err)
				return 1
			}

			syncRes, err := syncer.Sync()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to sync draft: %v\n", err)
				return 1
			}

			if _, err := metadata.UpdateSuggestionStatus(id, "applied", ""); err != nil {
				fmt.Fprintf(os.Stderr, "failed to update suggestion status: %v\n", err)
				return 1
			}

			res := output.ApplyResult{
				SuggestionID: id,
				Applied:      true,
				FilesChanged: filesChanged,
			}
			res.Draft = buildApplyDraft(syncRes.DraftSHA)
			if *checkpoint {
				message := ""
				if sug.Reason != "" {
					message = "fix: " + sug.Reason
				}
				checkpointRes, err := syncer.Checkpoint(message)
				if err != nil {
					fmt.Fprintf(os.Stderr, "checkpoint failed: %v\n", err)
					return 1
				}
				res.Checkpoint = checkpointRes.CheckpointSHA
				res.Draft = buildApplyDraft(checkpointRes.DraftSHA)
			}
			if !*checkpoint {
				res.NextActions = []output.ApplyAction{
					{Action: "checkpoint", Command: "jul checkpoint --json"},
				}
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			output.RenderApply(os.Stdout, res)
			return 0
		},
	}
}

func suggestionPatch(baseSHA, suggestedSHA string) (string, error) {
	if strings.TrimSpace(baseSHA) == "" || strings.TrimSpace(suggestedSHA) == "" {
		return "", fmt.Errorf("base and suggested commits required")
	}
	cmd := exec.Command("git", "diff", baseSHA, suggestedSHA)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %s", strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func applyPatch(patch string, force bool) error {
	if strings.TrimSpace(patch) == "" {
		return nil
	}
	args := []string{"apply"}
	if force {
		args = append(args, "--reject", "--whitespace=nowarn")
	}
	cmd := exec.Command("git", args...)
	cmd.Stdin = strings.NewReader(patch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git apply failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func buildApplyDraft(draftSHA string) output.ApplyDraft {
	sha := strings.TrimSpace(draftSHA)
	if sha == "" {
		return output.ApplyDraft{}
	}
	msg, _ := gitutil.CommitMessage(sha)
	changeID := gitutil.ExtractChangeID(msg)
	if changeID == "" {
		changeID = gitutil.FallbackChangeID(sha)
	}
	filesChanged := draftFilesChanged(sha)
	return output.ApplyDraft{
		ChangeID:     changeID,
		FilesChanged: filesChanged,
	}
}

func draftFilesChanged(draftSHA string) int {
	if strings.TrimSpace(draftSHA) == "" {
		return 0
	}
	if checkpoint, _ := latestCheckpoint(); checkpoint != nil {
		files, _ := diffNameOnly(checkpoint.SHA, draftSHA)
		return len(files)
	}
	if parent, err := gitutil.ParentOf(draftSHA); err == nil && strings.TrimSpace(parent) != "" {
		files, _ := diffNameOnly(parent, draftSHA)
		return len(files)
	}
	files, _ := diffNameOnly("", draftSHA)
	return len(files)
}

func diffNameOnly(from, to string) ([]string, error) {
	if strings.TrimSpace(to) == "" {
		return nil, nil
	}
	args := []string{"diff", "--name-only"}
	if strings.TrimSpace(from) != "" {
		args = append(args, from, to)
	} else {
		args = append(args, "--root", to)
	}
	cmd := exec.Command("git", args...)
	if root, err := gitutil.RepoTopLevel(); err == nil && root != "" {
		cmd.Dir = root
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %s", strings.TrimSpace(string(output)))
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, nil
	}
	lines := strings.Split(trimmed, "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		files = append(files, name)
	}
	return files, nil
}
