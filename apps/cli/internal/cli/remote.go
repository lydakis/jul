package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/output"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

type remoteOutput struct {
	Status     string   `json:"status"`
	Action     string   `json:"action,omitempty"`
	RemoteName string   `json:"remote_name,omitempty"`
	RemoteURL  string   `json:"remote_url,omitempty"`
	Remotes    []string `json:"remotes,omitempty"`
	Message    string   `json:"message,omitempty"`
}

func newRemoteCommand() Command {
	return Command{
		Name:    "remote",
		Summary: "Manage Jul remote selection",
		Run: func(args []string) int {
			jsonOut, args := stripJSONFlag(args)
			if len(args) == 0 {
				if jsonOut {
					args = ensureJSONFlag(args)
				}
				return runRemoteShow(args)
			}
			sub := args[0]
			subArgs := args[1:]
			if jsonOut {
				subArgs = ensureJSONFlag(subArgs)
			}
			switch sub {
			case "set":
				return runRemoteSet(subArgs)
			case "show":
				return runRemoteShow(subArgs)
			default:
				if jsonOut {
					_ = output.EncodeError(os.Stdout, "remote_unknown_subcommand", fmt.Sprintf("unknown subcommand %q", sub), nil)
					return 2
				}
				fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", sub)
				return 2
			}
		},
	}
}

func runRemoteSet(args []string) int {
	fs, jsonOut := newFlagSet("remote set")
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "remote_missing_name", "remote name required", nil)
		} else {
			fmt.Fprintln(os.Stderr, "remote name required")
		}
		return 2
	}
	if !gitutil.RemoteExists(name) {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "remote_not_found", fmt.Sprintf("remote %q not found; add it with git first", name), nil)
		} else {
			fmt.Fprintf(os.Stderr, "remote %q not found; add it with git first\n", name)
		}
		return 1
	}
	if err := config.SetRepoConfigValue("remote", "name", name); err != nil {
		if *jsonOut {
			_ = output.EncodeError(os.Stdout, "remote_config_failed", fmt.Sprintf("failed to write repo config: %v", err), nil)
		} else {
			fmt.Fprintf(os.Stderr, "failed to write repo config: %v\n", err)
		}
		return 1
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err == nil {
		if err := ensureJulRefspecs(repoRoot, name); err != nil {
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "remote_refspec_failed", fmt.Sprintf("failed to configure remote refspecs: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "failed to configure remote refspecs: %v\n", err)
			}
			return 1
		}
	}
	out := remoteOutput{
		Status:     "ok",
		Action:     "set",
		RemoteName: name,
		Message:    fmt.Sprintf("Now using remote %q for sync.", name),
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderRemoteOutput(out)
	return 0
}

func runRemoteShow(args []string) int {
	fs, jsonOut := newFlagSet("remote show")
	_ = fs.Parse(args)
	remote, err := remotesel.Resolve()
	if err != nil {
		switch err {
		case remotesel.ErrNoRemote:
			out := remoteOutput{
				Status:  "no_remote",
				Action:  "show",
				Message: "No git remotes configured.",
			}
			if *jsonOut {
				return writeJSON(out)
			}
			renderRemoteOutput(out)
			return 0
		case remotesel.ErrMultipleRemote:
			remotes, _ := gitutil.ListRemotes()
			names := make([]string, 0, len(remotes))
			for _, rem := range remotes {
				names = append(names, rem.Name)
			}
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "remote_multiple", "multiple remotes found; run 'jul remote set <name>'", nil)
			} else {
				if len(names) > 0 {
					fmt.Fprintf(os.Stdout, "Multiple remotes found: %s\n", strings.Join(names, ", "))
				}
				fmt.Fprintln(os.Stdout, "Run 'jul remote set <name>' to choose one.")
			}
			return 1
		default:
			if *jsonOut {
				_ = output.EncodeError(os.Stdout, "remote_resolve_failed", fmt.Sprintf("failed to resolve remote: %v", err), nil)
			} else {
				fmt.Fprintf(os.Stderr, "failed to resolve remote: %v\n", err)
			}
			return 1
		}
	}
	out := remoteOutput{
		Status:     "ok",
		Action:     "show",
		RemoteName: remote.Name,
		RemoteURL:  remote.URL,
	}
	if *jsonOut {
		return writeJSON(out)
	}
	renderRemoteOutput(out)
	return 0
}

func renderRemoteOutput(out remoteOutput) {
	if out.Message != "" {
		fmt.Fprintln(os.Stdout, out.Message)
		return
	}
	if out.RemoteName != "" && out.RemoteURL != "" {
		fmt.Fprintf(os.Stdout, "%s (%s)\n", out.RemoteName, out.RemoteURL)
		return
	}
	if out.RemoteName != "" {
		fmt.Fprintf(os.Stdout, "%s\n", out.RemoteName)
		return
	}
	if out.Status == "no_remote" {
		fmt.Fprintln(os.Stdout, "No git remotes configured.")
	}
}
