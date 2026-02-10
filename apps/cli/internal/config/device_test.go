package config

import (
	"path/filepath"
	"regexp"
	"testing"
)

func TestDeviceIDStableAndHighEntropy(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	id1, err := DeviceID()
	if err != nil {
		t.Fatalf("DeviceID failed: %v", err)
	}
	if matched := regexp.MustCompile(`^dev-[0-9a-f]{32}$`).MatchString(id1); !matched {
		t.Fatalf("expected high-entropy device id format, got %q", id1)
	}

	id2, err := DeviceID()
	if err != nil {
		t.Fatalf("DeviceID second call failed: %v", err)
	}
	if id2 != id1 {
		t.Fatalf("expected stable device id, got %q then %q", id1, id2)
	}
}
