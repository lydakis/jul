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

func newDiffCommand() Command {
	return Command{
		Name:    "diff",
		Summary: "Show diff between checkpoints or draft",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("diff", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stat := fs.Bool("stat", false, "Show diffstat only")
			nameOnly := fs.Bool("name-only", false, "Show filenames only")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			pos := fs.Args()
			from, to, rootDiff, err := resolveDiffTargets(pos)
			if err != nil {
				fmt.Fprintf(os.Stderr, "diff failed: %v\n", err)
				return 1
			}

			diffOut, err := runDiff(from, to, *stat, *nameOnly, rootDiff)
			if err != nil {
				fmt.Fprintf(os.Stderr, "diff failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				payload := output.DiffResult{From: from, To: to, Diff: diffOut}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(payload); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			output.RenderDiff(os.Stdout, output.DiffResult{From: from, To: to, Diff: diffOut})
			return 0
		},
	}
}

func resolveDiffTargets(args []string) (string, string, bool, error) {
	switch len(args) {
	case 0:
		user, workspace := workspaceParts()
		if workspace == "" {
			workspace = "@"
		}
		draftRef, err := syncRef(user, workspace)
		if err != nil {
			return "", "", false, err
		}
		draftSHA := ""
		if gitutil.RefExists(draftRef) {
			draftSHA, _ = gitutil.ResolveRef(draftRef)
		}
		if draftSHA == "" {
			draftSHA, _ = gitutil.Git("rev-parse", "HEAD")
		}
		last, err := latestCheckpoint()
		if err != nil {
			return "", "", false, err
		}
		if last == nil {
			return "", "", false, fmt.Errorf("no checkpoints found")
		}
		return last.SHA, draftSHA, false, nil
	case 1:
		id := strings.TrimSpace(args[0])
		if id == "" {
			return "", "", false, fmt.Errorf("commit or suggestion required")
		}
		if sug, ok, err := metadata.GetSuggestionByID(id); err == nil && ok {
			return sug.BaseCommitSHA, sug.SuggestedCommitSHA, false, nil
		} else if err != nil {
			return "", "", false, err
		}
		sha, err := gitutil.Git("rev-parse", id)
		if err != nil {
			return "", "", false, fmt.Errorf("failed to resolve %s", id)
		}
		parent, err := gitutil.Git("rev-parse", sha+"^")
		if err != nil {
			return "", sha, true, nil
		}
		return parent, sha, false, nil
	default:
		left := strings.TrimSpace(args[0])
		right := strings.TrimSpace(args[1])
		if left == "" || right == "" {
			return "", "", false, fmt.Errorf("two commits required")
		}
		from := left
		to := right
		if sug, ok, err := metadata.GetSuggestionByID(left); err == nil && ok {
			from = sug.BaseCommitSHA
			to = sug.SuggestedCommitSHA
		} else if err != nil {
			return "", "", false, err
		} else if sha, err := gitutil.Git("rev-parse", left); err == nil {
			from = sha
		} else {
			return "", "", false, fmt.Errorf("failed to resolve %s", left)
		}

		if sug, ok, err := metadata.GetSuggestionByID(right); err == nil && ok {
			to = sug.SuggestedCommitSHA
		} else if err != nil {
			return "", "", false, err
		} else if sha, err := gitutil.Git("rev-parse", right); err == nil {
			to = sha
		} else {
			return "", "", false, fmt.Errorf("failed to resolve %s", right)
		}
		return from, to, false, nil
	}
}

func runDiff(from, to string, stat, nameOnly bool, root bool) (string, error) {
	args := []string{"diff"}
	if stat {
		args = append(args, "--stat")
	}
	if nameOnly {
		args = append(args, "--name-only")
	}
	if root {
		args = append(args, "--root")
	}
	if strings.TrimSpace(from) != "" {
		args = append(args, from)
	}
	if strings.TrimSpace(to) != "" {
		args = append(args, to)
	}
	out, err := gitutil.Git(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
