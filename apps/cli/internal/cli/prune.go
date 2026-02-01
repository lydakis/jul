package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/notes"
	"github.com/lydakis/jul/cli/internal/output"
)

type keepRefRecord struct {
	Ref           string
	SHA           string
	User          string
	Workspace     string
	ChangeID      string
	CheckpointSHA string
}

type pruneOutput struct {
	Status             string `json:"status"`
	PrunedRefs         int    `json:"pruned_refs"`
	NotesRemoved       int    `json:"notes_removed"`
	SuggestionsRemoved int    `json:"suggestions_removed"`
	Message            string `json:"message,omitempty"`
}

func newPruneCommand() Command {
	return Command{
		Name:    "prune",
		Summary: "Remove expired keep-refs and related metadata",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("prune")
			_ = fs.Parse(args)

			days := config.RetentionCheckpointKeepDays()
			if days < 0 {
				out := pruneOutput{
					Status:  "ok",
					Message: "Retention disabled; nothing to prune.",
				}
				if *jsonOut {
					return writeJSON(out)
				}
				renderPruneOutput(out)
				return 0
			}
			cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

			keepRefs, err := listAllKeepRefs()
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "prune_failed", fmt.Sprintf("prune failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "prune failed: %v\n", err)
				}
				return 1
			}

			suggestions, err := metadata.ListSuggestions("", "", 0)
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "prune_failed", fmt.Sprintf("prune failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "prune failed: %v\n", err)
				}
				return 1
			}
			suggestionsByBase := map[string][]client.Suggestion{}
			for _, sug := range suggestions {
				suggestionsByBase[sug.BaseCommitSHA] = append(suggestionsByBase[sug.BaseCommitSHA], sug)
			}

			pruned := 0
			notesRemoved := 0
			suggestionsRemoved := 0
			for _, ref := range keepRefs {
				if !shouldExpireKeepRef(ref, cutoff) {
					continue
				}

				if isAnchorPinned(ref) {
					continue
				}

				if err := deleteRef(ref.Ref); err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "prune_delete_failed", fmt.Sprintf("failed to delete %s: %v", ref.Ref, err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to delete %s: %v\n", ref.Ref, err)
					}
					return 1
				}
				pruned++

				if err := notes.Remove(notes.RefAttestationsCheckpoint, ref.CheckpointSHA); err == nil {
					notesRemoved++
				}

				for _, sug := range suggestionsByBase[ref.CheckpointSHA] {
					if err := deleteRef(fmt.Sprintf("refs/jul/suggest/%s/%s", sug.ChangeID, sug.SuggestionID)); err == nil {
						suggestionsRemoved++
					}
					_ = notes.Remove(notes.RefSuggestions, sug.SuggestedCommitSHA)
				}

				if isAnchor(ref.ChangeID, ref.CheckpointSHA) {
					_ = notes.Remove(notes.RefCRComments, ref.CheckpointSHA)
					_ = notes.Remove(notes.RefCRState, ref.CheckpointSHA)
				}
			}

			out := pruneOutput{
				Status:             "ok",
				PrunedRefs:         pruned,
				NotesRemoved:       notesRemoved,
				SuggestionsRemoved: suggestionsRemoved,
				Message:            fmt.Sprintf("Pruned %d keep-ref(s), removed %d note(s), %d suggestion(s).", pruned, notesRemoved, suggestionsRemoved),
			}
			if *jsonOut {
				return writeJSON(out)
			}
			renderPruneOutput(out)
			return 0
		},
	}
}

func renderPruneOutput(out pruneOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
	}
}

func listAllKeepRefs() ([]keepRefRecord, error) {
	out, err := gitutil.Git("show-ref")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	records := make([]keepRefRecord, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sha := strings.TrimSpace(fields[0])
		ref := strings.TrimSpace(fields[1])
		if !strings.HasPrefix(ref, "refs/jul/keep/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(ref, "refs/jul/keep/"), "/")
		if len(parts) < 4 {
			continue
		}
		records = append(records, keepRefRecord{
			Ref:           ref,
			SHA:           sha,
			User:          parts[0],
			Workspace:     parts[1],
			ChangeID:      parts[2],
			CheckpointSHA: parts[3],
		})
	}
	return records, nil
}

func shouldExpireKeepRef(ref keepRefRecord, cutoff time.Time) bool {
	if cutoff.IsZero() {
		return false
	}
	commitTime, err := commitUnixTime(ref.CheckpointSHA)
	if err != nil {
		return false
	}
	return commitTime.Before(cutoff)
}

func commitUnixTime(sha string) (time.Time, error) {
	if strings.TrimSpace(sha) == "" {
		return time.Time{}, fmt.Errorf("commit sha required")
	}
	out, err := gitutil.Git("log", "-1", "--format=%ct", sha)
	if err != nil {
		return time.Time{}, err
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("missing commit time")
	}
	secs, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0).UTC(), nil
}

func deleteRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("ref required")
	}
	_, err := gitutil.Git("update-ref", "-d", ref)
	return err
}

func isAnchor(changeID, checkpointSHA string) bool {
	if strings.TrimSpace(changeID) == "" || strings.TrimSpace(checkpointSHA) == "" {
		return false
	}
	anchorSHA, err := gitutil.ResolveRef(anchorRef(changeID))
	if err != nil {
		return false
	}
	return strings.TrimSpace(anchorSHA) == strings.TrimSpace(checkpointSHA)
}

func isAnchorPinned(ref keepRefRecord) bool {
	if !isAnchor(ref.ChangeID, ref.CheckpointSHA) {
		return false
	}
	state, ok, err := metadata.ReadChangeRequestState(ref.CheckpointSHA)
	if err != nil || !ok {
		return false
	}
	return strings.ToLower(strings.TrimSpace(state.Status)) == "open"
}
