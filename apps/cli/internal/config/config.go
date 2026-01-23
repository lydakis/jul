package config

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
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

func BaseURLConfigured() bool {
	if strings.TrimSpace(os.Getenv(EnvBaseURL)) != "" {
		return true
	}
	if cfg := userConfigValue("server.url"); cfg != "" {
		return true
	}
	if cfg := userConfigValue("client.base_url"); cfg != "" {
		return true
	}
	if cfg := userConfigValue("base_url"); cfg != "" {
		return true
	}
	if cfg := gitConfigValue("jul.baseurl"); cfg != "" {
		return true
	}
	if cfg := gitConfigValue("jul.base_url"); cfg != "" {
		return true
	}
	return false
}

func WorkspaceID() string {
	if value := strings.TrimSpace(os.Getenv(EnvWorkspace)); value != "" {
		if strings.Contains(value, "/") {
			return value
		}
		user := UserName()
		if user != "" {
			return user + "/" + value
		}
		return value
	}
	if cfg := configValue("workspace.id"); cfg != "" {
		return cfg
	}
	if cfg := configValue("workspace"); cfg != "" {
		return cfg
	}
	if cfg := configValue("client.workspace"); cfg != "" {
		return cfg
	}
	if cfg := gitConfigValue("jul.workspace"); cfg != "" {
		return cfg
	}
	user := UserName()
	workspace := WorkspaceName()
	if user == "" {
		user = usernameFallback()
	}
	if workspace == "" {
		workspace = "@"
	}
	if user == "" {
		return workspace
	}
	return user + "/" + workspace
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
	if cfg := strings.TrimSpace(LoadAgentConfig().DefaultProvider); cfg != "" {
		return cfg
	}
	if cfg := configValue("agent.provider"); cfg != "" {
		return cfg
	}
	if cfg := configValue("agent"); cfg != "" {
		return cfg
	}
	if cfg := configValue("client.agent"); cfg != "" {
		return cfg
	}
	return ""
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

func UserName() string {
	if cfg := configValue("user.name"); cfg != "" {
		return cfg
	}
	if cfg := configValue("user"); cfg != "" {
		return cfg
	}
	if cfg := ServerUser(); cfg != "" {
		return cfg
	}
	return usernameFallback()
}

func WorkspaceName() string {
	if cfg := configValue("workspace.name"); cfg != "" {
		return cfg
	}
	if cfg := configValue("workspace.default_name"); cfg != "" {
		return cfg
	}
	return "@"
}

func RemoteName() string {
	if cfg := configValue("remote.name"); cfg != "" {
		return cfg
	}
	return ""
}

func RemoteURL() string {
	if cfg := configValue("remote.url"); cfg != "" {
		return cfg
	}
	return ""
}

func CIRunOnCheckpoint() bool {
	return configBool("ci.run_on_checkpoint", true)
}

func CIRunOnDraft() bool {
	return configBool("ci.run_on_draft", true)
}

func CIDraftBlocking() bool {
	return configBool("ci.draft_ci_blocking", false)
}

func ReviewEnabled() bool {
	return configBool("review.enabled", true)
}

func ReviewRunOnCheckpoint() bool {
	return configBool("review.run_on_checkpoint", true)
}

func PromoteTarget() string {
	if cfg := configValue("promote.default_target"); cfg != "" {
		return cfg
	}
	return "main"
}

func ReviewMinConfidence() float64 {
	return configFloat("review.min_confidence", 0)
}

func TraceSyncPromptHash() bool {
	return configBool("traces.sync_prompt_hash", true)
}

func TraceSyncPromptSummary() bool {
	return configBool("traces.sync_prompt_summary", false)
}

func TraceSyncPromptFull() bool {
	return configBool("traces.sync_prompt_full", false)
}

func CheckpointAdoptOnCommit() bool {
	return configBool("checkpoint.adopt_on_commit", false)
}

func CheckpointAdoptRunCI() bool {
	return configBool("checkpoint.adopt_run_ci", false)
}

func CheckpointAdoptRunReview() bool {
	return configBool("checkpoint.adopt_run_review", false)
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

func repoConfigValue(key string) string {
	path, err := repoConfigPath()
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

func configValue(key string) string {
	if cfg := repoConfigValue(key); cfg != "" {
		return cfg
	}
	if cfg := userConfigValue(key); cfg != "" {
		return cfg
	}
	return ""
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

func configBool(key string, def bool) bool {
	if cfg := configValue(key); cfg != "" {
		normalized := strings.ToLower(strings.TrimSpace(cfg))
		switch normalized {
		case "true", "yes", "1", "on":
			return true
		case "false", "no", "0", "off":
			return false
		}
	}
	return def
}

func configFloat(key string, def float64) float64 {
	if cfg := configValue(key); cfg != "" {
		if val, err := strconv.ParseFloat(strings.TrimSpace(cfg), 64); err == nil {
			return val
		}
	}
	return def
}

func repoConfigPath() (string, error) {
	root, err := repoTopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".jul", "config.toml"), nil
}

func repoTopLevel() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
