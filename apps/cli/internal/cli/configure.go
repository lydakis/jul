package cli

import (
	"fmt"
	"os"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/output"
)

type configureOutput struct {
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
	UserConfigPath  string `json:"user_config_path,omitempty"`
	AgentConfigPath string `json:"agent_config_path,omitempty"`
}

func newConfigureCommand() Command {
	return Command{
		Name:    "configure",
		Summary: "Run interactive configuration wizard",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("configure")
			_ = fs.Parse(args)

			cfg, err := config.RunWizard()
			if err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "configure_failed", fmt.Sprintf("failed to run wizard: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to run wizard: %v\n", err)
				}
				return 1
			}
			if err := config.WriteUserConfig(cfg); err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "configure_write_failed", fmt.Sprintf("failed to write config: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to write config: %v\n", err)
				}
				return 1
			}
			if err := config.WriteAgentConfig(cfg.Agent); err != nil {
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "configure_agent_failed", fmt.Sprintf("failed to write agent config: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "failed to write agent config: %v\n", err)
				}
				return 1
			}
			userPath, _ := config.UserConfigPath()
			agentPath := config.AgentConfigPath()
			out := configureOutput{
				Status:          "ok",
				Message:         "Configuration saved.",
				UserConfigPath:  userPath,
				AgentConfigPath: agentPath,
			}
			if *jsonOut {
				return writeJSON(out)
			}
			renderConfigureOutput(out)
			return 0
		},
	}
}

func renderConfigureOutput(out configureOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
	}
	if out.UserConfigPath != "" {
		fmt.Fprintf(os.Stdout, "User config: %s\n", out.UserConfigPath)
	}
	if out.AgentConfigPath != "" {
		fmt.Fprintf(os.Stdout, "Agent config: %s\n", out.AgentConfigPath)
	}
}
