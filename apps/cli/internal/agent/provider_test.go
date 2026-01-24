package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBundledOpenCodePathForSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on windows")
	}

	root := t.TempDir()
	caskDir := filepath.Join(root, "Caskroom", "jul", "0.0.1")
	libexecDir := filepath.Join(caskDir, "libexec", "jul")
	if err := os.MkdirAll(libexecDir, 0o755); err != nil {
		t.Fatalf("mkdir libexec: %v", err)
	}

	opencodePath := filepath.Join(libexecDir, "opencode")
	if err := os.WriteFile(opencodePath, []byte("ok"), 0o755); err != nil {
		t.Fatalf("write opencode: %v", err)
	}

	binPath := filepath.Join(caskDir, "jul")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write jul binary: %v", err)
	}

	linkDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	linkPath := filepath.Join(linkDir, "jul")
	if err := os.Symlink(binPath, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	got, err := bundledOpenCodePathFor(linkPath)
	if err != nil {
		t.Fatalf("expected opencode path, got error: %v", err)
	}
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(opencodePath)
	if gotResolved != wantResolved {
		t.Fatalf("expected %s, got %s", opencodePath, got)
	}
}
