package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeConflictResolution(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "merge-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	julPath := buildCLI(t)
	home := filepath.Join(t.TempDir(), "home")
	env := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
	}

	runCmd(t, repo, env, julPath, "init", "merge-repo")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, "conflict.txt", "base\n")
	runCmd(t, repo, env, julPath, "sync")

	checkpointOut := runCmd(t, repo, env, julPath, "checkpoint", "-m", "base", "--no-ci", "--no-review", "--json")
	var checkpointRes struct {
		CheckpointSHA string `json:"CheckpointSHA"`
	}
	if err := json.NewDecoder(strings.NewReader(checkpointOut)).Decode(&checkpointRes); err != nil {
		t.Fatalf("failed to decode checkpoint output: %v", err)
	}
	if checkpointRes.CheckpointSHA == "" {
		t.Fatalf("expected checkpoint sha")
	}

	runCmd(t, repo, nil, "git", "reset", "--hard", checkpointRes.CheckpointSHA)
	writeFile(t, repo, "conflict.txt", "ours\n")
	oursOut := runCmd(t, repo, env, julPath, "sync", "--json")
	var oursRes struct {
		DraftSHA     string `json:"DraftSHA"`
		WorkspaceRef string `json:"WorkspaceRef"`
		SyncRef      string `json:"SyncRef"`
	}
	if err := json.NewDecoder(strings.NewReader(oursOut)).Decode(&oursRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if oursRes.DraftSHA == "" || oursRes.SyncRef == "" || oursRes.WorkspaceRef == "" {
		t.Fatalf("expected draft/sync/workspace refs")
	}

	runCmd(t, repo, nil, "git", "reset", "--hard", checkpointRes.CheckpointSHA)
	writeFile(t, repo, "conflict.txt", "theirs\n")
	theirsOut := runCmd(t, repo, env, julPath, "sync", "--json")
	var theirsRes struct {
		DraftSHA string `json:"DraftSHA"`
	}
	if err := json.NewDecoder(strings.NewReader(theirsOut)).Decode(&theirsRes); err != nil {
		t.Fatalf("failed to decode sync output: %v", err)
	}
	if theirsRes.DraftSHA == "" {
		t.Fatalf("expected theirs draft sha")
	}

	runCmd(t, repo, nil, "git", "update-ref", oursRes.SyncRef, oursRes.DraftSHA)
	runCmd(t, repo, nil, "git", "update-ref", oursRes.WorkspaceRef, theirsRes.DraftSHA)
	leasePath := filepath.Join(repo, ".jul", "workspaces", "@", "lease")
	if err := os.WriteFile(leasePath, []byte(strings.TrimSpace(checkpointRes.CheckpointSHA)+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write workspace lease: %v", err)
	}

	agentPath := filepath.Join(t.TempDir(), "agent.sh")
	agentScript := `#!/bin/sh
set -e
cd "$JUL_AGENT_WORKSPACE"
printf "resolved\n" > conflict.txt
printf '{"version":1,"status":"completed","suggestions":[]}\n'
`
	if err := os.WriteFile(agentPath, []byte(agentScript), 0o755); err != nil {
		t.Fatalf("write agent script failed: %v", err)
	}
	envAgent := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
		"JUL_AGENT_CMD": agentPath,
	}

	mergeOut := runCmd(t, repo, envAgent, julPath, "merge", "--apply", "--json")
	var mergeRes struct {
		Merge struct {
			Status       string `json:"status"`
			SuggestionID string `json:"suggestion_id"`
			Applied      bool   `json:"applied"`
		} `json:"merge"`
	}
	if err := json.NewDecoder(strings.NewReader(mergeOut)).Decode(&mergeRes); err != nil {
		t.Fatalf("failed to decode merge output: %v", err)
	}
	if mergeRes.Merge.Status != "resolved" || mergeRes.Merge.SuggestionID == "" || !mergeRes.Merge.Applied {
		t.Fatalf("expected resolved/applied merge, got %+v", mergeRes.Merge)
	}

	resolved := runCmd(t, repo, nil, "git", "show", oursRes.SyncRef+":conflict.txt")
	if !strings.Contains(resolved, "resolved") {
		t.Fatalf("expected resolved content, got %s", resolved)
	}
}
