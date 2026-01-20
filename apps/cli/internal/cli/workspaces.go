package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
)

func newWorkspaceCommand() Command {
	return Command{
		Name:    "ws",
		Summary: "Manage workspaces",
		Run: func(args []string) int {
			if len(args) == 0 {
				fmt.Fprintln(os.Stdout, config.WorkspaceID())
				return 0
			}

			sub := args[0]
			switch sub {
			case "list":
				return runWorkspaceList(args[1:])
			case "set", "new":
				return runWorkspaceSet(args[1:])
			case "current":
				fmt.Fprintln(os.Stdout, config.WorkspaceID())
				return 0
			default:
				printWorkspaceUsage()
				return 1
			}
		},
	}
}

func runWorkspaceList(args []string) int {
	fs := flag.NewFlagSet("ws list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.Parse(args)

	cli := client.New(config.BaseURL())
	workspaces, err := cli.ListWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list workspaces: %v\n", err)
		return 1
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stdout, "No workspaces.")
		return 0
	}
	for _, ws := range workspaces {
		fmt.Fprintf(os.Stdout, "%s %s %s\n", ws.WorkspaceID, ws.Repo, ws.Branch)
	}
	return 0
}

func runWorkspaceSet(args []string) int {
	fs := flag.NewFlagSet("ws set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	user := fs.String("user", "", "Override user for workspace id")
	_ = fs.Parse(args)

	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "workspace name required")
		return 1
	}

	wsID := name
	if !strings.Contains(name, "/") {
		owner := strings.TrimSpace(*user)
		if owner == "" {
			owner = config.ServerUser()
		}
		if owner == "" {
			owner = strings.Split(config.WorkspaceID(), "/")[0]
		}
		wsID = owner + "/" + name
	}

	if err := runGitConfig("jul.workspace", wsID); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set workspace: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Workspace set to %s\n", wsID)
	return 0
}

func runGitConfig(key, value string) error {
	cmd := exec.Command("git", "config", key, value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git config %s: %s", key, strings.TrimSpace(string(output)))
	}
	return nil
}

func printWorkspaceUsage() {
	fmt.Fprintln(os.Stdout, "Usage: jul ws [list|set|new|current]")
}
