package ci

import "testing"

func TestRunCommandsPass(t *testing.T) {
	result, err := RunCommands([]string{"true"}, "")
	if err != nil {
		t.Fatalf("RunCommands failed: %v", err)
	}
	if result.Status != "pass" {
		t.Fatalf("expected pass, got %s", result.Status)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(result.Commands))
	}
	if result.Commands[0].ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.Commands[0].ExitCode)
	}
}

func TestRunCommandsFail(t *testing.T) {
	result, err := RunCommands([]string{"false"}, "")
	if err != nil {
		t.Fatalf("RunCommands failed: %v", err)
	}
	if result.Status != "fail" {
		t.Fatalf("expected fail, got %s", result.Status)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(result.Commands))
	}
	if result.Commands[0].ExitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
}
