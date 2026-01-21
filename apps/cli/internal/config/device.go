package config

import (
	"crypto/rand"
	"fmt"
	"math/big"
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
	adjective, err := randomWord(deviceAdjectives)
	if err != nil {
		return "", err
	}
	noun, err := randomWord(deviceNouns)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", adjective, noun), nil
}

func randomWord(words []string) (string, error) {
	if len(words) == 0 {
		return "", fmt.Errorf("word list is empty")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
	if err != nil {
		return "", err
	}
	return words[n.Int64()], nil
}

var deviceAdjectives = []string{
	"quiet",
	"swift",
	"bright",
	"gentle",
	"brave",
	"calm",
	"bold",
	"lucky",
	"mighty",
	"noble",
	"rapid",
	"solid",
}

var deviceNouns = []string{
	"tiger",
	"mountain",
	"river",
	"forest",
	"eagle",
	"fox",
	"lion",
	"hawk",
	"ocean",
	"valley",
	"stone",
	"ember",
}
