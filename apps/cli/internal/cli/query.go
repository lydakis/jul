package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/config"
)

func newQueryCommand() Command {
	return Command{
		Name:    "query",
		Summary: "Query commits by criteria",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("query", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			tests := fs.String("tests", "", "Filter by attestation status (pass|fail)")
			author := fs.String("author", "", "Filter by author")
			changeID := fs.String("change-id", "", "Filter by change ID")
			limit := fs.Int("limit", 20, "Max results")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			cli := client.New(config.BaseURL())
			results, err := cli.Query(*tests, *changeID, *author, *limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "query failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(results); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			if len(results) == 0 {
				fmt.Fprintln(os.Stdout, "No results.")
				return 0
			}
			for _, res := range results {
				line := firstLine(res.Message)
				status := res.AttestationStatus
				if status == "" {
					status = "unknown"
				}
				fmt.Fprintf(os.Stdout, "%s %s (%s)\n", res.CommitSHA, strings.TrimSpace(line), status)
			}
			return 0
		},
	}
}

func firstLine(message string) string {
	if message == "" {
		return ""
	}
	lines := strings.Split(message, "\n")
	return lines[0]
}
