package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInferDefaultCommandsGoWork(t *testing.T) {
	root := t.TempDir()
	work := `go 1.22

use (
  ./apps/cli
  ./apps/server
)
`
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte(work), 0o644); err != nil {
		t.Fatalf("write go.work failed: %v", err)
	}

	cmds := InferDefaultCommands(root)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "apps/cli") || !strings.Contains(cmds[1], "apps/server") {
		t.Fatalf("unexpected commands: %v", cmds)
	}
}

func TestInferDefaultCommandsGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/demo\n"), 0o644); err != nil {
		t.Fatalf("write go.mod failed: %v", err)
	}
	cmds := InferDefaultCommands(root)
	if len(cmds) != 1 || cmds[0] != "go test ./..." {
		t.Fatalf("unexpected commands: %v", cmds)
	}
}
