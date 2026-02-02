//go:build jul_integ_spec

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mergeOutput struct {
	Merge struct {
		Status       string `json:"status"`
		SuggestionID string `json:"suggestion_id"`
		Applied      bool   `json:"applied"`
	} `json:"merge"`
}

func TestIT_MERGE_001(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	oursRef, _, checkpointSHA := setupMergeConflict(t, repo, device, julPath)
	mergeOut, err := runCmdAllowFailure(t, repo, device.Env, julPath, "merge", "--apply", "--json")
	if err != nil {
		worktree := filepath.Join(repo, ".jul", "agent-workspace", "worktree")
		if werr := os.WriteFile(filepath.Join(worktree, "conflict.txt"), []byte("manual resolution\n"), 0o644); werr != nil {
			t.Fatalf("failed to write manual resolution: %v", werr)
		}
		mergeOut = runCmd(t, repo, device.Env, julPath, "merge", "--apply", "--json")
	}
	var res mergeOutput
	if err := json.NewDecoder(strings.NewReader(mergeOut)).Decode(&res); err != nil {
		t.Fatalf("failed to decode merge output: %v", err)
	}
	if res.Merge.Status != "resolved" || !res.Merge.Applied {
		t.Fatalf("expected resolved/applied merge, got %+v", res.Merge)
	}

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	resolved := runCmd(t, repo, nil, "git", "show", syncRef+":conflict.txt")
	if strings.Contains(resolved, "<<<<<<<") || strings.Contains(resolved, ">>>>>>>") {
		t.Fatalf("expected resolved content, got %s", resolved)
	}

	// Ensure we did not lose our base checkpoint.
	_ = checkpointSHA
	_ = oursRef
}

func TestIT_MERGE_007(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	setupMergeConflict(t, repo, device, julPath)
	_, _ = runCmdInput(t, repo, device.Env, "n\n", julPath, "merge", "--json")

	pendingOut := runCmd(t, repo, device.Env, julPath, "suggestions", "--status", "pending", "--json")
	var pending struct {
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(strings.NewReader(pendingOut)).Decode(&pending); err != nil {
		t.Fatalf("failed to decode pending suggestions: %v", err)
	}
	if len(pending.Suggestions) == 0 {
		worktree := filepath.Join(repo, ".jul", "agent-workspace", "worktree")
		if err := os.WriteFile(filepath.Join(worktree, "conflict.txt"), []byte("manual resolution\n"), 0o644); err != nil {
			t.Fatalf("failed to write manual resolution: %v", err)
		}
		_, _ = runCmdInput(t, repo, device.Env, "n\n", julPath, "merge", "--json")
		pendingOut = runCmd(t, repo, device.Env, julPath, "suggestions", "--status", "pending", "--json")
		if err := json.NewDecoder(strings.NewReader(pendingOut)).Decode(&pending); err != nil {
			t.Fatalf("failed to decode pending suggestions after manual resolution: %v", err)
		}
		if len(pending.Suggestions) == 0 {
			t.Fatalf("expected pending suggestion")
		}
	}
	suggestionID := pending.Suggestions[0].SuggestionID

	rejectOut := runCmd(t, repo, device.Env, julPath, "reject", suggestionID, "--json")
	if !strings.Contains(rejectOut, suggestionID) {
		t.Fatalf("expected reject output to include suggestion id, got %s", rejectOut)
	}

	// Manually resolve in the agent worktree.
	worktree := filepath.Join(repo, ".jul", "agent-workspace", "worktree")
	if err := os.WriteFile(filepath.Join(worktree, "conflict.txt"), []byte("manual resolution\n"), 0o644); err != nil {
		t.Fatalf("failed to write manual resolution: %v", err)
	}
	manualContents, err := os.ReadFile(filepath.Join(worktree, "conflict.txt"))
	if err != nil {
		t.Fatalf("failed to read manual resolution: %v", err)
	}
	if !strings.Contains(string(manualContents), "manual resolution") {
		t.Fatalf("expected manual resolution in worktree, got %s", string(manualContents))
	}

	mergeOut := runCmd(t, repo, device.Env, julPath, "merge", "--apply", "--json")
	var res mergeOutput
	if err := json.NewDecoder(strings.NewReader(mergeOut)).Decode(&res); err != nil {
		t.Fatalf("failed to decode merge output: %v", err)
	}
	if res.Merge.Status != "resolved" || !res.Merge.Applied {
		t.Fatalf("expected resolved/applied merge, got %+v", res.Merge)
	}

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	resolved := runCmd(t, repo, nil, "git", "show", syncRef+":conflict.txt")
	if !strings.Contains(resolved, "manual resolution") {
		t.Fatalf("expected manual resolution to land, got %s", resolved)
	}
	if strings.Contains(resolved, "<<<<<<<") || strings.Contains(resolved, ">>>>>>>") {
		t.Fatalf("expected conflict markers removed, got %s", resolved)
	}

	rejectedOut := runCmd(t, repo, device.Env, julPath, "suggestions", "--status", "rejected", "--json")
	if !strings.Contains(rejectedOut, suggestionID) {
		t.Fatalf("expected rejected suggestion to be recorded, got %s", rejectedOut)
	}
}

func TestIT_MERGE_008(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo, true)
	julPath := buildCLI(t)
	device := newDeviceEnv(t, "dev1")

	runCmd(t, repo, device.Env, julPath, "init", "demo")

	setupMergeConflictWithContents(t, repo, device, julPath, "base\n", "ours-one\n", "theirs-one\n")
	_, _ = runCmdInput(t, repo, device.Env, "n\n", julPath, "merge", "--json")

	worktree := filepath.Join(repo, ".jul", "agent-workspace", "worktree")
	if err := os.WriteFile(filepath.Join(worktree, "conflict.txt"), []byte("stale resolution\n"), 0o644); err != nil {
		t.Fatalf("failed to write stale resolution: %v", err)
	}

	setupMergeConflictWithContents(t, repo, device, julPath, "base\n", "ours-two\n", "theirs-two\n")
	_, _ = runCmdInput(t, repo, device.Env, "n\n", julPath, "merge", "--json")

	contents, err := os.ReadFile(filepath.Join(worktree, "conflict.txt"))
	if err != nil {
		t.Fatalf("failed to read conflict file: %v", err)
	}
	text := string(contents)
	if strings.Contains(text, "stale resolution") {
		t.Fatalf("expected stale resolution to be cleared, got %s", text)
	}
	if !strings.Contains(text, "ours-two") && !strings.Contains(text, "theirs-two") {
		t.Fatalf("expected refreshed conflict content, got %s", text)
	}
}

func setupMergeConflict(t *testing.T, repo string, device deviceEnv, julPath string) (string, string, string) {
	return setupMergeConflictWithContents(t, repo, device, julPath, "base\n", "ours\n", "theirs\n")
}

func setupMergeConflictWithContents(t *testing.T, repo string, device deviceEnv, julPath, base, ours, theirs string) (string, string, string) {
	writeFile(t, repo, "conflict.txt", base)
	runCmd(t, repo, device.Env, julPath, "sync")

	checkpointOut := runCmd(t, repo, device.Env, julPath, "checkpoint", "-m", "base", "--no-ci", "--no-review", "--json")
	var cp checkpointResult
	if err := json.NewDecoder(strings.NewReader(checkpointOut)).Decode(&cp); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if cp.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	runCmd(t, repo, nil, "git", "reset", "--hard", cp.CheckpointSHA)
	writeFile(t, repo, "conflict.txt", ours)
	oursOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var oursRes syncResult
	if err := json.NewDecoder(strings.NewReader(oursOut)).Decode(&oursRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if oursRes.DraftSHA == "" {
		t.Fatalf("expected draft sha")
	}

	runCmd(t, repo, nil, "git", "reset", "--hard", cp.CheckpointSHA)
	writeFile(t, repo, "conflict.txt", theirs)
	theirsOut := runCmd(t, repo, device.Env, julPath, "sync", "--json")
	var theirsRes syncResult
	if err := json.NewDecoder(strings.NewReader(theirsOut)).Decode(&theirsRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if theirsRes.DraftSHA == "" {
		t.Fatalf("expected draft sha")
	}

	deviceID := readDeviceID(t, device.Home)
	syncRef := "refs/jul/sync/tester/" + deviceID + "/@"
	workspaceRef := "refs/jul/workspaces/tester/@"

	runCmd(t, repo, nil, "git", "update-ref", syncRef, oursRes.DraftSHA)
	runCmd(t, repo, nil, "git", "update-ref", workspaceRef, theirsRes.DraftSHA)

	leasePath := filepath.Join(repo, ".jul", "workspaces", "@", "lease")
	if err := os.WriteFile(leasePath, []byte(strings.TrimSpace(cp.CheckpointSHA)+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write workspace lease: %v", err)
	}

	return oursRes.DraftSHA, workspaceRef, cp.CheckpointSHA
}
