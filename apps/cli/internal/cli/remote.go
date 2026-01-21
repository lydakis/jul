package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/gitutil"
	remotesel "github.com/lydakis/jul/cli/internal/remote"
)

func newRemoteCommand() Command {
	return Command{
		Name:    "remote",
		Summary: "Manage Jul remote selection",
		Run: func(args []string) int {
			if len(args) == 0 {
				return runRemoteShow()
			}
			switch args[0] {
			case "set":
				return runRemoteSet(args[1:])
			case "show":
				return runRemoteShow()
			default:
				fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", args[0])
				return 2
			}
		},
	}
}

func runRemoteSet(args []string) int {
	fs := flag.NewFlagSet("remote set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.Parse(args)
	name := strings.TrimSpace(fs.Arg(0))
	if name == "" {
		fmt.Fprintln(os.Stderr, "remote name required")
		return 2
	}
	if !gitutil.RemoteExists(name) {
		fmt.Fprintf(os.Stderr, "remote %q not found; add it with git first\n", name)
		return 1
	}
	if err := config.SetRepoConfigValue("remote", "name", name); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write repo config: %v\n", err)
		return 1
	}
	repoRoot, err := gitutil.RepoTopLevel()
	if err == nil {
		if err := ensureJulRefspecs(repoRoot, name); err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure remote refspecs: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(os.Stdout, "Now using remote %q for sync.\n", name)
	return 0
}

func runRemoteShow() int {
	remote, err := remotesel.Resolve()
	if err != nil {
		switch err {
		case remotesel.ErrNoRemote:
			fmt.Fprintln(os.Stdout, "No git remotes configured.")
			return 0
		case remotesel.ErrMultipleRemote:
			remotes, _ := gitutil.ListRemotes()
			names := make([]string, 0, len(remotes))
			for _, rem := range remotes {
				names = append(names, rem.Name)
			}
			if len(names) > 0 {
				fmt.Fprintf(os.Stdout, "Multiple remotes found: %s\n", strings.Join(names, ", "))
			}
			fmt.Fprintln(os.Stdout, "Run 'jul remote set <name>' to choose one.")
			return 1
		default:
			fmt.Fprintf(os.Stderr, "failed to resolve remote: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(os.Stdout, "%s (%s)\n", remote.Name, remote.URL)
	return 0
}
