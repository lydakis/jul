package ci

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
)

type Config struct {
	Commands []CommandSpec
}

type CommandSpec struct {
	Name    string
	Command string
}

func ConfigPath() (string, error) {
	root, err := gitutil.RepoTopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".jul", "ci.toml"), nil
}

func LoadConfig() (Config, bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, false, err
	}
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
		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		if value == "" || name == "" {
			continue
		}
		cfg.Commands = append(cfg.Commands, CommandSpec{
			Name:    name,
			Command: value,
		})
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

func WriteConfig(commands []CommandSpec) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("[commands]\n")
	for i, cmd := range commands {
		name := strings.TrimSpace(cmd.Name)
		if name == "" {
			name = fmt.Sprintf("cmd%d", i+1)
		}
		value := strings.TrimSpace(cmd.Command)
		if value == "" {
			continue
		}
		b.WriteString(name)
		b.WriteString(" = ")
		b.WriteString(strconv.Quote(value))
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
