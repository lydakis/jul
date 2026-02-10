package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lydakis/jul/cli/internal/output"
)

func TestCIRunFailsWhenDeviceIDUnavailable(t *testing.T) {
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	writeFilePath(t, repo, "README.md", "ci\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "ci test")

	cwd, _ := os.Getwd()
	_ = os.Chdir(repo)
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	homeFile := filepath.Join(t.TempDir(), "home-file")
	if err := os.WriteFile(homeFile, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("failed to prepare blocked home path: %v", err)
	}
	t.Setenv("HOME", homeFile)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCIRunWithStream([]string{"--cmd", "true", "--target", "HEAD", "--json"}, nil, &out, &errOut, "", "manual")
	if code == 0 {
		t.Fatalf("expected ci run to fail when device id cannot be resolved")
	}

	var payload output.ErrorOutput
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON error output, got %q (%v)", out.String(), err)
	}
	if payload.Code != "ci_device_id_failed" {
		t.Fatalf("expected ci_device_id_failed, got %+v", payload)
	}
}
