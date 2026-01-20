package config

import (
	"os"
	"os/exec"
	"os/user"
	"strings"
)

const (
	EnvBaseURL   = "JUL_BASE_URL"
	EnvWorkspace = "JUL_WORKSPACE"
)

func BaseURL() string {
	value := strings.TrimSpace(os.Getenv(EnvBaseURL))
	if value == "" {
		if cfg := userConfigValue("base_url"); cfg != "" {
			return strings.TrimRight(cfg, "/")
		}
		if cfg := gitConfigValue("jul.baseurl"); cfg != "" {
			return strings.TrimRight(cfg, "/")
		}
		if cfg := gitConfigValue("jul.base_url"); cfg != "" {
			return strings.TrimRight(cfg, "/")
		}
		return "http://localhost:8000"
	}
	return strings.TrimRight(value, "/")
}

func WorkspaceID() string {
	if value := strings.TrimSpace(os.Getenv(EnvWorkspace)); value != "" {
		return value
	}
	if cfg := userConfigValue("workspace"); cfg != "" {
		return cfg
	}
	if cfg := gitConfigValue("jul.workspace"); cfg != "" {
		return cfg
	}

	name := "user"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	host := hostnameFallback()
	return name + "/" + host
}

func RepoName() string {
	if cfg := gitConfigValue("jul.reponame"); cfg != "" {
		return cfg
	}
	if cfg := gitConfigValue("jul.repo"); cfg != "" {
		return cfg
	}
	return ""
}

func DefaultAgent() string {
	if cfg := userConfigValue("agent"); cfg != "" {
		return cfg
	}
	return ""
}

func hostnameFallback() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "workspace"
	}
	return host
}

func gitConfigValue(key string) string {
	cmd := exec.Command("git", "config", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func userConfigValue(key string) string {
	path, err := userConfigPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		if k != key {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		return value
	}
	return ""
}
