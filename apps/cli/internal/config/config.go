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
