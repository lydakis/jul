package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	BaseRef string
	BaseSHA string
}

func ConfigPath(repoRoot, workspace string) string {
	return filepath.Join(repoRoot, ".jul", "workspaces", workspace, "config")
}

func ReadConfig(repoRoot, workspace string) (Config, bool, error) {
	path := ConfigPath(repoRoot, workspace)
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

func WriteConfig(repoRoot, workspace string, cfg Config) error {
	path := ConfigPath(repoRoot, workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := renderConfig(cfg)
	return os.WriteFile(path, []byte(content), 0o644)
}

func parseConfig(raw string) Config {
	cfg := Config{}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"")
		switch key {
		case "base_ref":
			cfg.BaseRef = val
		case "base_sha":
			cfg.BaseSHA = val
		}
	}
	return cfg
}

func renderConfig(cfg Config) string {
	var b strings.Builder
	if strings.TrimSpace(cfg.BaseRef) != "" {
		b.WriteString("base_ref = \"")
		b.WriteString(cfg.BaseRef)
		b.WriteString("\"\n")
	}
	if strings.TrimSpace(cfg.BaseSHA) != "" {
		b.WriteString("base_sha = \"")
		b.WriteString(cfg.BaseSHA)
		b.WriteString("\"\n")
	}
	return b.String()
}
