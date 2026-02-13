package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestCIRunManualTargetPersistsLatestCompletedWhenDraftHasNoResult(t *testing.T) {
	_, baseSHA, headSHA := setupCIManualTargetRepo(t)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runCIRunWithStream([]string{"--cmd", "true", "--target", baseSHA, "--json"}, nil, &out, &errOut, "", "manual")
	if code != 0 {
		t.Fatalf("manual ci run failed with code %d (stdout=%s stderr=%s)", code, out.String(), errOut.String())
	}

	statusOut, statusCode := captureStdoutWithCode(t, func() int {
		return runCIStatus([]string{"--json"})
	})
	if statusCode != 1 {
		t.Fatalf("expected stale ci status to return non-zero exit code, got %d (output=%s)", statusCode, statusOut)
	}
	var status output.CIStatusJSON
	if err := json.NewDecoder(strings.NewReader(statusOut)).Decode(&status); err != nil {
		t.Fatalf("failed to decode ci status output: %v (%s)", err, statusOut)
	}
	if strings.TrimSpace(status.CI.CurrentDraftSHA) != headSHA {
		t.Fatalf("expected current draft sha %s, got %+v", headSHA, status.CI)
	}
	if strings.TrimSpace(status.CI.CompletedSHA) != baseSHA {
		t.Fatalf("expected completed sha %s, got %+v", baseSHA, status.CI)
	}
	if strings.TrimSpace(status.CI.Status) != "stale" {
		t.Fatalf("expected stale status for non-draft completed result, got %+v", status.CI)
	}
	if status.CI.ResultsCurrent {
		t.Fatalf("expected results_current=false for non-draft target, got %+v", status.CI)
	}
}

func TestCIRunManualNonDraftTargetDoesNotReplaceCurrentDraftResult(t *testing.T) {
	_, baseSHA, headSHA := setupCIManualTargetRepo(t)

	var firstOut bytes.Buffer
	var firstErr bytes.Buffer
	code := runCIRunWithStream([]string{"--cmd", "true", "--target", headSHA, "--json"}, nil, &firstOut, &firstErr, "", "manual")
	if code != 0 {
		t.Fatalf("manual ci run for draft sha failed with code %d (stdout=%s stderr=%s)", code, firstOut.String(), firstErr.String())
	}

	var secondOut bytes.Buffer
	var secondErr bytes.Buffer
	code = runCIRunWithStream([]string{"--cmd", "true", "--target", baseSHA, "--json"}, nil, &secondOut, &secondErr, "", "manual")
	if code != 0 {
		t.Fatalf("manual ci run for non-draft sha failed with code %d (stdout=%s stderr=%s)", code, secondOut.String(), secondErr.String())
	}

	statusOut, statusCode := captureStdoutWithCode(t, func() int {
		return runCIStatus([]string{"--json"})
	})
	if statusCode != 0 {
		t.Fatalf("expected current-draft ci status to return success, got %d (output=%s)", statusCode, statusOut)
	}
	var status output.CIStatusJSON
	if err := json.NewDecoder(strings.NewReader(statusOut)).Decode(&status); err != nil {
		t.Fatalf("failed to decode ci status output: %v (%s)", err, statusOut)
	}
	if strings.TrimSpace(status.CI.CurrentDraftSHA) != headSHA {
		t.Fatalf("expected current draft sha %s, got %+v", headSHA, status.CI)
	}
	if strings.TrimSpace(status.CI.CompletedSHA) != headSHA {
		t.Fatalf("expected completed sha to remain current draft %s, got %+v", headSHA, status.CI)
	}
	if strings.TrimSpace(status.CI.Status) != "pass" {
		t.Fatalf("expected pass status to stay tied to current draft result, got %+v", status.CI)
	}
	if !status.CI.ResultsCurrent {
		t.Fatalf("expected results_current=true, got %+v", status.CI)
	}
}

func setupCIManualTargetRepo(t *testing.T) (string, string, string) {
	t.Helper()
	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")

	writeFilePath(t, repo, "README.md", "v1\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "base")
	baseSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	writeFilePath(t, repo, "README.md", "v2\n")
	runGitCmd(t, repo, "add", "README.md")
	runGitCmd(t, repo, "commit", "-m", "head")
	headSHA := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))

	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	t.Setenv("HOME", t.TempDir())

	return repo, baseSHA, headSHA
}

func captureStdoutWithCode(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w
	code := fn()
	_ = w.Close()
	os.Stdout = orig
	out, _ := io.ReadAll(r)
	return string(out), code
}
