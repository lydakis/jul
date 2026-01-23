package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WizardConfig struct {
	RemoteURL  string
	RemoteName string
	User       string
	Workspace  string
	Agent      string
}

func RunWizard() (WizardConfig, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Remote URL (optional, leave blank for local-only): ")
	remoteURL, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	remoteURL = strings.TrimSpace(remoteURL)

	fmt.Print("Remote name (default: origin): ")
	remoteName, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		remoteName = "origin"
	}

	fmt.Print("Username: ")
	user, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	user = strings.TrimSpace(user)

	fmt.Print("Default workspace name (e.g. @): ")
	workspace, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "@"
	}

	fmt.Print("Default agent provider (opencode|codex|custom): ")
	agent, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	agent = strings.TrimSpace(agent)

	return WizardConfig{
		RemoteURL:  remoteURL,
		RemoteName: remoteName,
		User:       user,
		Workspace:  workspace,
		Agent:      agent,
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

	content := "[user]\n"
	if cfg.User != "" {
		content += fmt.Sprintf("name = %q\n", cfg.User)
	}
	content += "\n[remote]\n"
	if cfg.RemoteName != "" {
		content += fmt.Sprintf("name = %q\n", cfg.RemoteName)
	}
	if cfg.RemoteURL != "" {
		content += fmt.Sprintf("url = %q\n", cfg.RemoteURL)
	}
	content += "\n[workspace]\n"
	if cfg.Workspace != "" {
		content += fmt.Sprintf("default_name = %q\n", cfg.Workspace)
	}
	content += "\n[agent]\n"
	if cfg.Agent != "" {
		content += fmt.Sprintf("provider = %q\n", cfg.Agent)
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
