package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmokeLocalOnlyFlow(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	julPath := buildCLI(t)
	home := filepath.Join(t.TempDir(), "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "demo")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	// Sync draft locally
	writeFile(t, repo, "README.md", "hello\n")
	syncOut := runCmd(t, repo, env, julPath, "sync", "--json")
	var syncRes struct {
		DraftSHA     string `json:"DraftSHA"`
		WorkspaceRef string `json:"WorkspaceRef"`
		SyncRef      string `json:"SyncRef"`
	}
	if err := json.NewDecoder(strings.NewReader(syncOut)).Decode(&syncRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if syncRes.DraftSHA == "" || syncRes.WorkspaceRef == "" || syncRes.SyncRef == "" {
		t.Fatalf("expected sync refs, got %+v", syncRes)
	}
	runCmd(t, repo, nil, "git", "show-ref", syncRes.SyncRef)
	runCmd(t, repo, nil, "git", "show-ref", syncRes.WorkspaceRef)

	// Create an explicit trace with prompt metadata.
	traceOut := runCmd(t, repo, env, julPath, "trace", "--prompt", "write a trace", "--json")
	var traceRes struct {
		TraceSHA   string `json:"trace_sha"`
		PromptHash string `json:"prompt_hash"`
	}
	if err := json.NewDecoder(strings.NewReader(traceOut)).Decode(&traceRes); err != nil {
		t.Fatalf("failed to decode trace output: %v", err)
	}
	if traceRes.TraceSHA == "" {
		t.Fatalf("expected trace sha")
	}
	traceNote := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/traces", "show", traceRes.TraceSHA)
	if !strings.Contains(traceNote, traceRes.PromptHash) {
		t.Fatalf("expected trace note prompt hash, got %s", traceNote)
	}

	// Checkpoint locally (keep-ref)
	checkpointOut := runCmd(t, repo, env, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review", "--json")
	var checkpointRes struct {
		CheckpointSHA string `json:"CheckpointSHA"`
		KeepRef       string `json:"KeepRef"`
	}
	if err := json.NewDecoder(strings.NewReader(checkpointOut)).Decode(&checkpointRes); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if checkpointRes.KeepRef == "" || checkpointRes.CheckpointSHA == "" {
		t.Fatalf("expected keep ref and checkpoint sha")
	}
	runCmd(t, repo, nil, "git", "show-ref", checkpointRes.KeepRef)

	agentPath := filepath.Join(t.TempDir(), "agent.sh")
	agentScript := `#!/bin/sh
set -e
cd "$JUL_AGENT_WORKSPACE"
git config user.name "Agent"
git config user.email "agent@example.com"
echo "agent change" >> README.md
git add README.md
git commit -m "agent suggestion" >/dev/null
sha=$(git rev-parse HEAD)
printf '{"version":1,"status":"completed","suggestions":[{"commit":"%s","reason":"review","description":"agent change","confidence":0.9}]}\n' "$sha"
`
	if err := os.WriteFile(agentPath, []byte(agentScript), 0o755); err != nil {
		t.Fatalf("write agent script failed: %v", err)
	}
	envAgent := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
		"JUL_AGENT_CMD": agentPath,
	}

	reviewOut := runCmd(t, repo, envAgent, julPath, "review", "--json")
	var reviewRes struct {
		Suggestions []struct {
			SuggestionID string `json:"suggestion_id"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(strings.NewReader(reviewOut)).Decode(&reviewRes); err != nil {
		t.Fatalf("failed to decode review output: %v", err)
	}
	if len(reviewRes.Suggestions) == 0 || reviewRes.Suggestions[0].SuggestionID == "" {
		t.Fatalf("expected suggestions from review")
	}
	suggestionID := reviewRes.Suggestions[0].SuggestionID

	suggestionsOut := runCmd(t, repo, envAgent, julPath, "suggestions", "--json")
	var suggestions []struct {
		SuggestionID string `json:"suggestion_id"`
		Status       string `json:"status"`
	}
	if err := json.NewDecoder(strings.NewReader(suggestionsOut)).Decode(&suggestions); err != nil {
		t.Fatalf("failed to decode suggestions output: %v", err)
	}
	if len(suggestions) == 0 || suggestions[0].SuggestionID == "" {
		t.Fatalf("expected suggestion entries")
	}

	showSugOut := runCmd(t, repo, envAgent, julPath, "show", "--json", suggestionID)
	var showSugRes struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(strings.NewReader(showSugOut)).Decode(&showSugRes); err != nil {
		t.Fatalf("failed to decode suggestion show output: %v", err)
	}
	if showSugRes.Type != "suggestion" {
		t.Fatalf("expected suggestion show output")
	}

	diffSugOut := runCmd(t, repo, envAgent, julPath, "diff", "--json", suggestionID)
	var diffSugRes struct {
		Diff string `json:"diff"`
	}
	if err := json.NewDecoder(strings.NewReader(diffSugOut)).Decode(&diffSugRes); err != nil {
		t.Fatalf("failed to decode suggestion diff: %v", err)
	}
	if strings.TrimSpace(diffSugRes.Diff) == "" {
		t.Fatalf("expected suggestion diff output")
	}

	applyOut := runCmd(t, repo, envAgent, julPath, "apply", "--json", suggestionID)
	var applyRes struct {
		SuggestionID string `json:"suggestion_id"`
		Applied      bool   `json:"applied"`
	}
	if err := json.NewDecoder(strings.NewReader(applyOut)).Decode(&applyRes); err != nil {
		t.Fatalf("failed to decode apply output: %v", err)
	}
	if !applyRes.Applied || applyRes.SuggestionID == "" {
		t.Fatalf("expected apply result")
	}

	appliedOut := runCmd(t, repo, envAgent, julPath, "suggestions", "--status", "applied", "--json")
	var appliedRes []struct {
		SuggestionID string `json:"suggestion_id"`
	}
	if err := json.NewDecoder(strings.NewReader(appliedOut)).Decode(&appliedRes); err != nil {
		t.Fatalf("failed to decode applied suggestions: %v", err)
	}
	if len(appliedRes) == 0 {
		t.Fatalf("expected applied suggestions")
	}

	ciOut := runCmd(t, repo, env, julPath, "ci", "run", "--cmd", "true", "--target", checkpointRes.CheckpointSHA, "--json")
	var ciRes struct {
		CI struct {
			Status string `json:"status"`
		} `json:"ci"`
	}
	if err := json.NewDecoder(strings.NewReader(ciOut)).Decode(&ciRes); err != nil {
		t.Fatalf("failed to decode ci output: %v", err)
	}
	if ciRes.CI.Status == "" {
		t.Fatalf("expected ci status")
	}
	listOut := runCmd(t, repo, env, julPath, "ci", "list", "--json")
	var listRes struct {
		Runs []struct {
			ID        string `json:"id"`
			CommitSHA string `json:"commit_sha"`
			Status    string `json:"status"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(strings.NewReader(listOut)).Decode(&listRes); err != nil {
		t.Fatalf("failed to decode ci list output: %v", err)
	}
	if len(listRes.Runs) == 0 || listRes.Runs[0].ID == "" {
		t.Fatalf("expected ci runs list")
	}
	note := runCmd(t, repo, nil, "git", "notes", "--ref", "refs/notes/jul/attestations/checkpoint", "show", checkpointRes.CheckpointSHA)
	if !strings.Contains(note, "\"status\"") {
		t.Fatalf("expected attestation note, got %s", note)
	}

	statusOut := runCmd(t, repo, env, julPath, "status", "--json")
	var statusRes struct {
		WorkspaceID string `json:"workspace_id"`
		DraftSHA    string `json:"draft_sha"`
		ChangeID    string `json:"change_id"`
		WorkingTree struct {
			Untracked []struct {
				Path string `json:"path"`
			} `json:"untracked"`
		} `json:"working_tree"`
	}
	if err := json.NewDecoder(strings.NewReader(statusOut)).Decode(&statusRes); err != nil {
		t.Fatalf("failed to decode status output: %v", err)
	}
	if statusRes.DraftSHA == "" || statusRes.ChangeID == "" {
		t.Fatalf("expected status draft/change, got %+v", statusRes)
	}
	if len(statusRes.WorkingTree.Untracked) == 0 {
		t.Fatalf("expected working tree untracked entries")
	}

	logOut := runCmd(t, repo, env, julPath, "log", "--json")
	var logRes []struct {
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(logOut)).Decode(&logRes); err != nil {
		t.Fatalf("failed to decode log output: %v", err)
	}
	if len(logRes) == 0 || logRes[0].CommitSHA == "" {
		t.Fatalf("expected log entries")
	}

	showOut := runCmd(t, repo, env, julPath, "show", "--json", checkpointRes.CheckpointSHA)
	var showRes struct {
		Type      string `json:"type"`
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(showOut)).Decode(&showRes); err != nil {
		t.Fatalf("failed to decode show output: %v", err)
	}
	if showRes.Type != "checkpoint" || showRes.CommitSHA == "" {
		t.Fatalf("expected checkpoint show output")
	}

	blameOut := runCmd(t, repo, env, julPath, "blame", "README.md", "--json")
	var blameRes struct {
		File  string `json:"file"`
		Lines []struct {
			Line             int    `json:"line"`
			Content          string `json:"content"`
			CheckpointSHA    string `json:"checkpoint_sha"`
			CheckpointChange string `json:"checkpoint_change_id"`
		} `json:"lines"`
	}
	if err := json.NewDecoder(strings.NewReader(blameOut)).Decode(&blameRes); err != nil {
		t.Fatalf("failed to decode blame output: %v", err)
	}
	if blameRes.File != "README.md" || len(blameRes.Lines) == 0 {
		t.Fatalf("expected blame lines for README.md")
	}
	if blameRes.Lines[0].CheckpointSHA == "" || blameRes.Lines[0].CheckpointChange == "" {
		t.Fatalf("expected blame checkpoint metadata")
	}

	queryOut := runCmd(t, repo, env, julPath, "query", "--limit", "5", "--json")
	var queryRes []struct {
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(queryOut)).Decode(&queryRes); err != nil {
		t.Fatalf("failed to decode query output: %v", err)
	}
	if len(queryRes) == 0 || queryRes[0].CommitSHA == "" {
		t.Fatalf("expected query results")
	}

	diffOut := runCmd(t, repo, env, julPath, "diff", "--json")
	var diffRes struct {
		Diff string `json:"diff"`
	}
	if err := json.NewDecoder(strings.NewReader(diffOut)).Decode(&diffRes); err != nil {
		t.Fatalf("failed to decode diff output: %v", err)
	}
	if strings.TrimSpace(diffRes.Diff) == "" {
		t.Fatalf("expected diff output")
	}

	ciStatusOut, _ := runCmdAllowFailure(t, repo, env, julPath, "ci", "status", "--json")
	var ciStatusRes struct {
		CI struct {
			CompletedSHA string `json:"completed_sha"`
		} `json:"ci"`
	}
	if err := json.NewDecoder(strings.NewReader(ciStatusOut)).Decode(&ciStatusRes); err != nil {
		t.Fatalf("failed to decode ci status output: %v", err)
	}
	if ciStatusRes.CI.CompletedSHA == "" {
		t.Fatalf("expected ci completed sha")
	}

	runCmd(t, repo, env, julPath, "ci", "run", "--watch", "--cmd", "true")

	remoteOut := runCmd(t, repo, env, julPath, "remote", "show")
	if !strings.Contains(remoteOut, "No git remotes configured") {
		t.Fatalf("expected remote show to indicate no remotes, got %s", remoteOut)
	}

	wsOut := runCmd(t, repo, env, julPath, "ws")
	if strings.TrimSpace(wsOut) == "" {
		t.Fatalf("expected workspace output")
	}

	wsListOut := runCmd(t, repo, env, julPath, "ws", "list")
	if strings.TrimSpace(wsListOut) == "" {
		t.Fatalf("expected ws list output")
	}
	// Promote locally by anchoring to the checkpoint SHA so HEAD doesn't need to exist.
	promoteOut := runCmd(t, repo, env, julPath, "promote", "--to", "main", checkpointRes.CheckpointSHA)
	if strings.TrimSpace(promoteOut) == "" {
		t.Fatalf("expected promote output")
	}
	checkpointMsg := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", checkpointRes.CheckpointSHA)
	oldChangeID := extractChangeID(checkpointMsg)
	if oldChangeID == "" {
		t.Fatalf("expected Change-Id on checkpoint")
	}
	workspaceRef := fmt.Sprintf("refs/jul/workspaces/%s/%s", "tester", "@")
	newDraftSHA := strings.TrimSpace(runCmd(t, repo, nil, "git", "rev-parse", workspaceRef))
	if newDraftSHA == "" {
		t.Fatalf("expected workspace ref after promote")
	}
	if newDraftSHA == checkpointRes.CheckpointSHA {
		t.Fatalf("expected new draft after promote")
	}
	newDraftMsg := runCmd(t, repo, nil, "git", "log", "-1", "--format=%B", newDraftSHA)
	newChangeID := extractChangeID(newDraftMsg)
	if newChangeID == "" || newChangeID == oldChangeID {
		t.Fatalf("expected new Change-Id after promote")
	}
	changesOut := runCmd(t, repo, env, julPath, "changes")
	if strings.TrimSpace(changesOut) == "" {
		t.Fatalf("expected changes output")
	}

	mainRef := runCmd(t, repo, nil, "git", "show-ref", "refs/heads/main")
	if strings.TrimSpace(mainRef) == "" {
		t.Fatalf("expected main ref to be created by promote")
	}

	reflogOut := runCmd(t, repo, env, julPath, "reflog", "--json")
	var reflogRes []struct {
		CommitSHA string `json:"commit_sha"`
	}
	if err := json.NewDecoder(strings.NewReader(reflogOut)).Decode(&reflogRes); err != nil {
		t.Fatalf("failed to decode reflog output: %v", err)
	}

	runCmd(t, repo, env, julPath, "ws", "checkout", "@")
	leasePath := filepath.Join(repo, ".jul", "workspaces", "@", "lease")
	if _, err := os.Stat(leasePath); err != nil {
		t.Fatalf("expected workspace lease file: %v", err)
	}
	deviceID, err := os.ReadFile(filepath.Join(home, ".config", "jul", "device"))
	if err != nil {
		t.Fatalf("failed to read device id: %v", err)
	}
	syncRef := fmt.Sprintf("refs/jul/sync/%s/%s/%s", "tester", strings.TrimSpace(string(deviceID)), "@")
	runCmd(t, repo, nil, "git", "show-ref", syncRef)
}

func extractChangeID(message string) string {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Change-Id:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Change-Id:"))
		}
	}
	return ""
}
