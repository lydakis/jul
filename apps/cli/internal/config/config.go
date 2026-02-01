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
	EnvWorkspace = "JUL_WORKSPACE"
)

func WorkspaceID() string {
	if value := strings.TrimSpace(os.Getenv(EnvWorkspace)); value != "" {
		if strings.Contains(value, "/") {
			return value
		}
		user := UserNamespace()
		if user == "" {
			user = UserName()
		}
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
	user := UserNamespace()
	if user == "" {
		user = UserName()
	}
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

func UserNamespace() string {
	if cfg := configValue("user.user_namespace"); cfg != "" {
		return cfg
	}
	if cfg := configValue("user_namespace"); cfg != "" {
		return cfg
	}
	return ""
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

func DraftSyncEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(configValue("remote.draft_sync")))
	switch value {
	case "disabled", "false", "0", "off", "no":
		return false
	case "enabled", "true", "1", "on", "yes":
		return true
	}
	return true
}

func SyncAutoRestack() bool {
	if cfg := configValue("sync.autorestack"); cfg != "" {
		if val, err := strconv.ParseBool(cfg); err == nil {
			return val
		}
	}
	return true
}

func SyncMode() string {
	mode := strings.ToLower(strings.TrimSpace(configValue("sync.mode")))
	switch mode {
	case "continuous", "explicit", "on-command":
		return mode
	}
	if mode == "" {
		return "on-command"
	}
	return "on-command"
}

func SyncDebounceSeconds() int {
	return configInt("sync.debounce_seconds", 2)
}

func SyncMinIntervalSeconds() int {
	return configInt("sync.min_interval_seconds", 5)
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

func CISyncOutput() bool {
	return configBool("ci.sync_output", false)
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

func TraceRunOnTrace() bool {
	return configBool("ci.run_on_trace", true)
}

func TraceChecks() []string {
	return configList("ci.trace_checks", []string{"lint", "typecheck"})
}

func AllowDraftSecrets() bool {
	return configBool("sync.allow_secrets", false)
}

func CheckpointAdoptOnCommit() bool {
	return configBool("checkpoint.adopt_on_commit", false)
}

func configList(key string, def []string) []string {
	raw := strings.TrimSpace(configValue(key))
	if raw == "" {
		return def
	}
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		trimmed = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
	}
	var parts []string
	if strings.Contains(trimmed, ",") {
		parts = strings.Split(trimmed, ",")
	} else {
		parts = []string{trimmed}
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		value = strings.Trim(value, "\"")
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func CheckpointAdoptRunCI() bool {
	return configBool("checkpoint.adopt_run_ci", false)
}

func CheckpointAdoptRunReview() bool {
	return configBool("checkpoint.adopt_run_review", false)
}

func RetentionCheckpointKeepDays() int {
	return configInt("retention.checkpoint_keep_days", 90)
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

func configInt(key string, def int) int {
	if cfg := configValue(key); cfg != "" {
		if val, err := strconv.Atoi(strings.TrimSpace(cfg)); err == nil {
			return val
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
