//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/output"
)

func TestIT_SUGG_004(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")
	setupMergeConflict(t, repo, device, julPath)
	_, _ = runCmdInput(t, repo, device.Env, "n\n", julPath, "merge", "--json")

	pending := ensurePendingSuggestion(t, repo, device.Env, julPath)
	reason := "conflict resolved manually"
	rejectOut := runCmd(t, repo, device.Env, julPath, "reject", pending.SuggestionID, "-m", reason, "--json")
	var rejected client.Suggestion
	if err := json.NewDecoder(strings.NewReader(rejectOut)).Decode(&rejected); err != nil {
		t.Fatalf("failed to decode reject output: %v (%s)", err, rejectOut)
	}
	if rejected.SuggestionID != pending.SuggestionID {
		t.Fatalf("expected rejected suggestion %s, got %+v", pending.SuggestionID, rejected)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("expected rejected status, got %+v", rejected)
	}
	if strings.TrimSpace(rejected.ResolutionMessage) != reason {
		t.Fatalf("expected reject reason %q, got %+v", reason, rejected)
	}

	rejectedView := listSuggestionsByStatus(t, repo, device.Env, julPath, "rejected")
	found := false
	for _, sug := range rejectedView.Suggestions {
		if sug.SuggestionID != pending.SuggestionID {
			continue
		}
		found = true
		if sug.Status != "rejected" {
			t.Fatalf("expected rejected suggestion status in listing, got %+v", sug)
		}
		if strings.TrimSpace(sug.ResolutionMessage) != reason {
			t.Fatalf("expected durable reject reason %q, got %+v", reason, sug)
		}
	}
	if !found {
		t.Fatalf("expected rejected suggestion %s in list, got %+v", pending.SuggestionID, rejectedView.Suggestions)
	}

	noteRaw := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/suggestions", "show", rejected.SuggestedCommitSHA)
	var noted client.Suggestion
	if err := json.NewDecoder(strings.NewReader(noteRaw)).Decode(&noted); err != nil {
		t.Fatalf("failed to decode suggestion note: %v (%s)", err, noteRaw)
	}
	if noted.SuggestionID != pending.SuggestionID {
		t.Fatalf("expected note suggestion %s, got %+v", pending.SuggestionID, noted)
	}
	if strings.TrimSpace(noted.ResolutionMessage) != reason {
		t.Fatalf("expected note to keep reject reason %q, got %+v", reason, noted)
	}
}

func ensurePendingSuggestion(t *testing.T, repo string, env map[string]string, julPath string) client.Suggestion {
	t.Helper()
	view := listSuggestionsByStatus(t, repo, env, julPath, "pending")
	if len(view.Suggestions) > 0 {
		return view.Suggestions[0]
	}

	worktree := filepath.Join(repo, ".jul", "agent-workspace", "worktree")
	if err := os.WriteFile(filepath.Join(worktree, "conflict.txt"), []byte("manual resolution\n"), 0o644); err != nil {
		t.Fatalf("failed to write manual resolution: %v", err)
	}
	_, _ = runCmdInput(t, repo, env, "n\n", julPath, "merge", "--json")
	view = listSuggestionsByStatus(t, repo, env, julPath, "pending")
	if len(view.Suggestions) == 0 {
		t.Fatalf("expected pending suggestion, got %+v", view)
	}
	return view.Suggestions[0]
}

func listSuggestionsByStatus(t *testing.T, repo string, env map[string]string, julPath string, status string) output.SuggestionsView {
	t.Helper()
	out := runCmd(t, repo, env, julPath, "suggestions", "--status", status, "--json")
	var view output.SuggestionsView
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&view); err != nil {
		t.Fatalf("failed to decode suggestions output: %v (%s)", err, out)
	}
	return view
}
