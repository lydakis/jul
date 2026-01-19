package main

import (
	"os"

	"github.com/lydakis/jul/cli/internal/cli"
)

const version = "0.0.1"

func main() {
	app := &cli.App{
		Commands: cli.Commands(version),
		Version:  version,
	}
	os.Exit(app.Run(os.Args[1:]))
}
