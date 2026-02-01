package syncignore

import (
	"os"
	"path/filepath"
	"strings"
)

var defaultPatterns = []string{
	".jul/",
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"id_rsa",
	".aws/credentials",
	".npmrc",
	".pypirc",
	".netrc",
}

func Load(repoRoot string) []string {
	patterns := append([]string{}, defaultPatterns...)
	if strings.TrimSpace(repoRoot) == "" {
		return patterns
	}
	path := filepath.Join(repoRoot, ".jul", "syncignore")
	data, err := os.ReadFile(path)
	if err != nil {
		return patterns
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, trimmed)
	}
	return patterns
}

func Match(path string, patterns []string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	path = filepath.ToSlash(trimmed)
	for _, pattern := range patterns {
		pat := strings.TrimSpace(pattern)
		if pat == "" || strings.HasPrefix(pat, "#") {
			continue
		}
		pat = filepath.ToSlash(pat)
		if strings.HasSuffix(pat, "/") {
			prefix := strings.TrimSuffix(pat, "/")
			if strings.HasPrefix(path, prefix+"/") || path == prefix {
				return true
			}
			continue
		}
		if strings.Contains(pat, "/") {
			if ok, _ := filepath.Match(pat, path); ok {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(pat, filepath.Base(path)); ok {
			return true
		}
	}
	return false
}
