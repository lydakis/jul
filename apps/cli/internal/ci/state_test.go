package ci

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")

	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	running := Running{
		CommitSHA: "abc123",
		PID:       4321,
		StartedAt: time.Now().UTC(),
	}
	if err := WriteRunning(running); err != nil {
		t.Fatalf("WriteRunning failed: %v", err)
	}
	gotRunning, err := ReadRunning()
	if err != nil {
		t.Fatalf("ReadRunning failed: %v", err)
	}
	if gotRunning == nil || gotRunning.CommitSHA != running.CommitSHA || gotRunning.PID != running.PID {
		t.Fatalf("unexpected running state: %+v", gotRunning)
	}
	if err := ClearRunning(); err != nil {
		t.Fatalf("ClearRunning failed: %v", err)
	}
	afterClear, err := ReadRunning()
	if err != nil {
		t.Fatalf("ReadRunning after clear failed: %v", err)
	}
	if afterClear != nil {
		t.Fatalf("expected running state to be cleared")
	}

	status := Status{
		CommitSHA: "def456",
		Result: Result{
			Status: "pass",
		},
		CompletedAt: time.Now().UTC(),
	}
	if err := WriteCompleted(status); err != nil {
		t.Fatalf("WriteCompleted failed: %v", err)
	}
	readStatus, err := ReadCompleted()
	if err != nil {
		t.Fatalf("ReadCompleted failed: %v", err)
	}
	if readStatus == nil || readStatus.CommitSHA != status.CommitSHA || readStatus.Result.Status != status.Result.Status {
		t.Fatalf("unexpected completed status: %+v", readStatus)
	}
}

func TestMarkRunCanceledByPID(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")

	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	pid := 9876
	started := time.Now().UTC().Add(-2 * time.Second)
	run := RunRecord{
		ID:        "run-1",
		CommitSHA: "abc123",
		Status:    "running",
		Mode:      "draft",
		StartedAt: started,
		PID:       pid,
	}
	if err := WriteRun(run); err != nil {
		t.Fatalf("WriteRun failed: %v", err)
	}

	if err := MarkRunCanceledByPID(pid); err != nil {
		t.Fatalf("MarkRunCanceledByPID failed: %v", err)
	}

	got, err := ReadRun("run-1")
	if err != nil {
		t.Fatalf("ReadRun failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected run to exist")
	}
	if got.Status != "canceled" {
		t.Fatalf("expected canceled status, got %+v", got)
	}
	if got.FinishedAt.IsZero() {
		t.Fatalf("expected canceled run to have finished_at")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(output))
	}
}
