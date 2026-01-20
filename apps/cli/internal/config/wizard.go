package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WizardConfig struct {
	BaseURL   string
	Workspace string
	Agent     string
}

func RunWizard() (WizardConfig, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Jul server URL (e.g. http://localhost:8000): ")
	baseURL, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	baseURL = strings.TrimSpace(baseURL)

	fmt.Print("Workspace id (user/name): ")
	workspace, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	workspace = strings.TrimSpace(workspace)

	fmt.Print("Default agent provider (opencode|codex|custom): ")
	agent, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	agent = strings.TrimSpace(agent)

	return WizardConfig{
		BaseURL:   baseURL,
		Workspace: workspace,
		Agent:     agent,
	}, nil
}

func WriteUserConfig(cfg WizardConfig) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content := "[client]\n"
	if cfg.BaseURL != "" {
		content += fmt.Sprintf("base_url = %q\n", cfg.BaseURL)
	}
	if cfg.Workspace != "" {
		content += fmt.Sprintf("workspace = %q\n", cfg.Workspace)
	}
	if cfg.Agent != "" {
		content += fmt.Sprintf("agent = %q\n", cfg.Agent)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func userConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "jul", "config.toml"), nil
}
