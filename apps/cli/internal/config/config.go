package config

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
)

const (
	EnvBaseURL   = "JUL_BASE_URL"
	EnvWorkspace = "JUL_WORKSPACE"
)

func BaseURL() string {
	value := strings.TrimSpace(os.Getenv(EnvBaseURL))
	if value == "" {
		if cfg := userConfigValue("server.url"); cfg != "" {
			return strings.TrimRight(cfg, "/")
		}
		if cfg := userConfigValue("client.base_url"); cfg != "" {
			return strings.TrimRight(cfg, "/")
		}
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
	if cfg := userConfigValue("workspace.id"); cfg != "" {
		return cfg
	}
	if cfg := userConfigValue("workspace"); cfg != "" {
		return cfg
	}
	if cfg := userConfigValue("client.workspace"); cfg != "" {
		return cfg
	}
	if cfg := gitConfigValue("jul.workspace"); cfg != "" {
		return cfg
	}
	if cfg := userConfigValue("workspace.default_name"); cfg != "" {
		user := ServerUser()
		if user == "" {
			user = usernameFallback()
		}
		if user != "" {
			return user + "/" + cfg
		}
	}

	user := usernameFallback()
	host := hostnameFallback()
	return user + "/" + host
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
	if cfg := userConfigValue("agent.provider"); cfg != "" {
		return cfg
	}
	if cfg := userConfigValue("agent"); cfg != "" {
		return cfg
	}
	if cfg := userConfigValue("client.agent"); cfg != "" {
		return cfg
	}
	return ""
}

func CreateRemoteDefault() bool {
	if cfg := userConfigValue("init.create_remote"); cfg != "" {
		if parsed, err := strconv.ParseBool(cfg); err == nil {
			return parsed
		}
	}
	if cfg := userConfigValue("create_remote"); cfg != "" {
		if parsed, err := strconv.ParseBool(cfg); err == nil {
			return parsed
		}
	}
	if cfg := userConfigValue("client.create_remote"); cfg != "" {
		if parsed, err := strconv.ParseBool(cfg); err == nil {
			return parsed
		}
	}
	return true
}

func ServerUser() string {
	if cfg := userConfigValue("server.user"); cfg != "" {
		return cfg
	}
	if cfg := userConfigValue("user"); cfg != "" {
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

func usernameFallback() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "user"
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
	config := parseUserConfig(string(data))
	return config[key]
}

func parseUserConfig(raw string) map[string]string {
	config := map[string]string{}
	section := ""
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		fullKey := key
		if section != "" {
			fullKey = section + "." + key
		}
		config[fullKey] = value
	}
	return config
}
