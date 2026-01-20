package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WizardConfig struct {
	BaseURL      string
	User         string
	Workspace    string
	Agent        string
	CreateRemote bool
}

func RunWizard() (WizardConfig, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Jul server URL (e.g. http://localhost:8000): ")
	baseURL, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	baseURL = strings.TrimSpace(baseURL)

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

	fmt.Print("Create remote repo by default? [Y/n]: ")
	createRemoteRaw, err := reader.ReadString('\n')
	if err != nil {
		return WizardConfig{}, err
	}
	createRemoteRaw = strings.TrimSpace(createRemoteRaw)
	createRemote := true
	if createRemoteRaw != "" {
		switch strings.ToLower(createRemoteRaw) {
		case "y", "yes", "true":
			createRemote = true
		case "n", "no", "false":
			createRemote = false
		}
	}

	return WizardConfig{
		BaseURL:      baseURL,
		User:         user,
		Workspace:    workspace,
		Agent:        agent,
		CreateRemote: createRemote,
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
	content = "[server]\n"
	if cfg.BaseURL != "" {
		content += fmt.Sprintf("url = %q\n", cfg.BaseURL)
	}
	if cfg.User != "" {
		content += fmt.Sprintf("user = %q\n", cfg.User)
	}
	content += "\n[workspace]\n"
	if cfg.Workspace != "" {
		content += fmt.Sprintf("default_name = %q\n", cfg.Workspace)
	}
	content += "\n[agent]\n"
	if cfg.Agent != "" {
		content += fmt.Sprintf("provider = %q\n", cfg.Agent)
	}
	content += "\n[init]\n"
	content += fmt.Sprintf("create_remote = %t\n", cfg.CreateRemote)
	return os.WriteFile(path, []byte(content), 0o644)
}

func userConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "jul", "config.toml"), nil
}
