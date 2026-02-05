package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutoSyncRunsInBackground(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	runCmd(t, repo, nil, "git", "init")
	runCmd(t, repo, nil, "git", "config", "user.name", "Test User")
	runCmd(t, repo, nil, "git", "config", "user.email", "test@example.com")

	writeFile(t, repo, "README.md", "hello\n")
	runCmd(t, repo, nil, "git", "add", "README.md")
	runCmd(t, repo, nil, "git", "commit", "-m", "feat: base")

	julPath := buildCLI(t)
	home := filepath.Join(tmp, "home")
	envNoSync := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
		"JUL_NO_SYNC":   "1",
	}
	envAuto := map[string]string{
		"HOME":          home,
		"JUL_WORKSPACE": "tester/@",
		"JUL_NO_SYNC":   "",
	}

	runCmd(t, repo, envNoSync, julPath, "init", "demo")
	runCmd(t, repo, envNoSync, julPath, "checkpoint", "-m", "feat: first", "--no-ci", "--no-review")

	syncDir := filepath.Join(repo, ".jul", "sync")
	_ = os.RemoveAll(syncDir)

	runCmd(t, repo, envAuto, julPath, "status", "--json")

	completedPath := filepath.Join(syncDir, "completed.json")

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(completedPath); err == nil {
			var res struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(data, &res); err != nil {
				t.Fatalf("failed to decode completed.json: %v", err)
			}
			if res.ID == "" {
				t.Fatalf("expected completed sync id")
			}
			if res.Status == "" {
				t.Fatalf("expected completed sync status")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("expected auto-sync to record completion metadata")
}
