package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".jul", "workspaces", "@"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	cfg := Config{
		BaseRef:  "refs/heads/main",
		BaseSHA:  "abc123",
		TrackRef: "refs/heads/main",
		TrackTip: "def456",
	}
	if err := WriteConfig(root, "@", cfg); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	read, ok, err := ReadConfig(root, "@")
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected config to exist")
	}
	if read.BaseRef != cfg.BaseRef {
		t.Fatalf("expected base_ref %s, got %s", cfg.BaseRef, read.BaseRef)
	}
	if read.BaseSHA != cfg.BaseSHA {
		t.Fatalf("expected base_sha %s, got %s", cfg.BaseSHA, read.BaseSHA)
	}
	if read.TrackRef != cfg.TrackRef {
		t.Fatalf("expected track_ref %s, got %s", cfg.TrackRef, read.TrackRef)
	}
	if read.TrackTip != cfg.TrackTip {
		t.Fatalf("expected track_tip %s, got %s", cfg.TrackTip, read.TrackTip)
	}
}
