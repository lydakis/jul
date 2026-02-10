package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

func DeviceID() (string, error) {
	path, err := devicePath()
	if err != nil {
		return "", err
	}
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id, nil
		}
	}
	id, err := generateDeviceID()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func devicePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "jul", "device"), nil
}

func generateDeviceID() (string, error) {
	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return "dev-" + hex.EncodeToString(buf), nil
}
