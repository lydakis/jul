package cli

import (
	"fmt"
	"os"
	"strings"
)

type Command struct {
	Name    string
	Summary string
	Run     func(args []string) int
}

type App struct {
	Commands []Command
	Version  string
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		return a.usage("missing command")
	}

	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return a.usage("")
	}

	for _, cmd := range a.Commands {
		if cmd.Name == args[0] {
			return cmd.Run(args[1:])
		}
	}

	return a.usage(fmt.Sprintf("unknown command: %s", args[0]))
}

func (a *App) usage(problem string) int {
	if problem != "" {
		fmt.Fprintf(os.Stderr, "error: %s\n\n", problem)
	}

	fmt.Fprintf(os.Stderr, "jul %s\n", a.Version)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  jul <command> [options]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	for _, cmd := range a.Commands {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", cmd.Name, cmd.Summary)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jul help' to see this message.")

	if problem == "" {
		return 0
	}
	return 1
}

func FormatLines(lines ...string) string {
	return strings.Join(lines, "\n")
}
