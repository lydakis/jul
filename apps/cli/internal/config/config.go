package config

import (
	"os"
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
		return "http://localhost:8000"
	}
	return strings.TrimRight(value, "/")
}

func WorkspaceID() string {
	if value := strings.TrimSpace(os.Getenv(EnvWorkspace)); value != "" {
		return value
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
