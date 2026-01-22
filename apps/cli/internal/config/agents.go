package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AgentProvider struct {
	Name          string
	Command       string
	Headless      string
	Protocol      string
	Mode          string
	Timeout       time.Duration
	Bundled       bool
	MaxIterations int
	EnableNetwork bool
	Actions       map[string]string
}

type AgentConfig struct {
	DefaultProvider string
	Providers       map[string]AgentProvider
}

func LoadAgentConfig() AgentConfig {
	cfg := AgentConfig{
		DefaultProvider: "opencode",
		Providers:       map[string]AgentProvider{},
	}
	data, err := os.ReadFile(agentConfigPath())
	if err != nil {
		ensureDefaultProvider(&cfg)
		return cfg
	}
	parsed := parseUserConfig(string(data))
	if provider := strings.TrimSpace(parsed["default.provider"]); provider != "" {
		cfg.DefaultProvider = provider
	}
	for key, value := range parsed {
		if !strings.HasPrefix(key, "providers.") {
			continue
		}
		rest := strings.TrimPrefix(key, "providers.")
		parts := strings.Split(rest, ".")
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		provider := cfg.Providers[name]
		provider.Name = name
		if len(parts) >= 4 && parts[1] == "actions" {
			action := parts[2]
			field := parts[3]
			if field == "headless" {
				if provider.Actions == nil {
					provider.Actions = map[string]string{}
				}
				provider.Actions[action] = strings.TrimSpace(value)
			}
			cfg.Providers[name] = provider
			continue
		}
		if len(parts) != 2 {
			continue
		}
		field := parts[1]
		switch field {
		case "command":
			provider.Command = strings.TrimSpace(value)
		case "headless":
			provider.Headless = strings.TrimSpace(value)
		case "protocol":
			provider.Protocol = strings.TrimSpace(value)
		case "mode":
			provider.Mode = strings.TrimSpace(value)
		case "bundled":
			provider.Bundled = parseBool(value)
		case "timeout_seconds":
			if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
				provider.Timeout = time.Duration(seconds) * time.Second
			}
		case "max_iterations":
			if iter, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && iter > 0 {
				provider.MaxIterations = iter
			}
		case "enable_network":
			provider.EnableNetwork = parseBool(value)
		}
		cfg.Providers[name] = provider
	}
	ensureDefaultProvider(&cfg)
	return cfg
}

func AgentProviderConfig(name string) (AgentProvider, bool) {
	cfg := LoadAgentConfig()
	provider, ok := cfg.Providers[name]
	return provider, ok
}

func DefaultAgentProvider() AgentProvider {
	cfg := LoadAgentConfig()
	provider, ok := cfg.Providers[cfg.DefaultProvider]
	if ok {
		return provider
	}
	return defaultBundledProvider()
}

func WriteAgentConfig(defaultProvider string) error {
	path := agentConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	provider := strings.TrimSpace(defaultProvider)
	if provider == "" {
		provider = "opencode"
	}
	content := "[default]\n"
	content += "provider = " + strconv.Quote(provider) + "\n\n"
	content += "[providers.opencode]\n"
	content += "command = \"opencode\"\n"
	content += "bundled = true\n"
	content += "protocol = \"jul-agent-v1\"\n"
	content += "mode = \"prompt\"\n"
	content += "headless = \"opencode run --format json --file $ATTACHMENT $PROMPT\"\n"
	content += "timeout_seconds = 300\n"
	content += "\n[providers.codex]\n"
	content += "command = \"codex\"\n"
	content += "bundled = false\n"
	content += "protocol = \"jul-agent-v1\"\n"
	content += "mode = \"prompt\"\n"
	content += "headless = \"codex exec --output-format json --full-auto $PROMPT\"\n"
	content += "timeout_seconds = 300\n"
	content += "\n[providers.claude-code]\n"
	content += "command = \"claude\"\n"
	content += "bundled = false\n"
	content += "protocol = \"jul-agent-v1\"\n"
	content += "mode = \"prompt\"\n"
	content += "headless = \"claude -p $PROMPT --output-format json --permission-mode acceptEdits\"\n"
	content += "timeout_seconds = 300\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func agentConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "agents.toml"
	}
	return filepath.Join(home, ".config", "jul", "agents.toml")
}

func ensureDefaultProvider(cfg *AgentConfig) {
	if cfg.Providers == nil {
		cfg.Providers = map[string]AgentProvider{}
	}
	if _, ok := cfg.Providers["opencode"]; !ok {
		cfg.Providers["opencode"] = defaultBundledProvider()
	}
}

func defaultBundledProvider() AgentProvider {
	return AgentProvider{
		Name:     "opencode",
		Command:  "opencode",
		Bundled:  true,
		Protocol: "jul-agent-v1",
		Mode:     "prompt",
		Timeout:  5 * time.Minute,
	}
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1", "on":
		return true
	default:
		return false
	}
}
