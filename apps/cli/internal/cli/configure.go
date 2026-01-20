package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/lydakis/jul/cli/internal/config"
)

func newConfigureCommand() Command {
	return Command{
		Name:    "configure",
		Summary: "Run interactive configuration wizard",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("configure", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			_ = fs.Parse(args)

			cfg, err := config.RunWizard()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to run wizard: %v\n", err)
				return 1
			}
			if err := config.WriteUserConfig(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write config: %v\n", err)
				return 1
			}
			fmt.Fprintln(os.Stdout, "Configuration saved.")
			return 0
		},
	}
}
