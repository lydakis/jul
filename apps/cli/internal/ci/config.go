package ci

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

type Config struct {
	Commands []string
}

func LoadConfig() (Config, bool, error) {
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return Config{}, false, err
	}
	path := filepath.Join(root, ".jul", "ci.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	cfg := parseConfig(string(data))
	return cfg, true, nil
}

func parseConfig(raw string) Config {
	cfg := Config{}
	section := ""
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = stripInlineComment(trimmed)
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		if section != "commands" {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		if value == "" {
			continue
		}
		cfg.Commands = append(cfg.Commands, value)
	}
	return cfg
}

func stripInlineComment(line string) string {
	inQuotes := false
	for i, r := range line {
		if r == '"' {
			inQuotes = !inQuotes
			continue
		}
		if r == '#' && !inQuotes {
			return line[:i]
		}
	}
	return line
}
