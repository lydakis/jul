package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

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
			compiles := fs.String("compiles", "", "Filter by compile status (true|false)")
			coverageMin := fs.Float64("coverage-min", -1, "Minimum coverage percentage")
			coverageMax := fs.Float64("coverage-max", -1, "Maximum coverage percentage")
			author := fs.String("author", "", "Filter by author")
			changeID := fs.String("change-id", "", "Filter by change ID")
			since := fs.String("since", "", "Only commits after RFC3339 time")
			until := fs.String("until", "", "Only commits before RFC3339 time")
			limit := fs.Int("limit", 20, "Max results")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			var compilesFilter *bool
			if strings.TrimSpace(*compiles) != "" {
				switch strings.ToLower(strings.TrimSpace(*compiles)) {
				case "true", "pass", "yes":
					value := true
					compilesFilter = &value
				case "false", "fail", "no":
					value := false
					compilesFilter = &value
				default:
					fmt.Fprintln(os.Stderr, "compiles must be true or false")
					return 1
				}
			}

			var coverageMinFilter *float64
			if *coverageMin >= 0 {
				coverageMinFilter = coverageMin
			}

			var coverageMaxFilter *float64
			if *coverageMax >= 0 {
				coverageMaxFilter = coverageMax
			}

			var sinceFilter *time.Time
			if strings.TrimSpace(*since) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*since))
				if err != nil {
					fmt.Fprintln(os.Stderr, "since must be RFC3339")
					return 1
				}
				sinceFilter = &parsed
			}

			var untilFilter *time.Time
			if strings.TrimSpace(*until) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*until))
				if err != nil {
					fmt.Fprintln(os.Stderr, "until must be RFC3339")
					return 1
				}
				untilFilter = &parsed
			}

			cli := client.New(config.BaseURL())
			results, err := cli.Query(client.QueryFilters{
				Tests:       strings.TrimSpace(*tests),
				Compiles:    compilesFilter,
				CoverageMin: coverageMinFilter,
				CoverageMax: coverageMaxFilter,
				ChangeID:    strings.TrimSpace(*changeID),
				Author:      strings.TrimSpace(*author),
				Since:       sinceFilter,
				Until:       untilFilter,
				Limit:       *limit,
			})
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
				status := res.TestStatus
				if status == "" {
					status = res.AttestationStatus
				}
				if status == "" {
					status = "unknown"
				}
				coverage := ""
				if res.CoverageLinePct != nil {
					coverage = fmt.Sprintf(", %.1f%% coverage", *res.CoverageLinePct)
				}
				fmt.Fprintf(os.Stdout, "%s %s (%s%s)\n", res.CommitSHA, strings.TrimSpace(line), status, coverage)
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
