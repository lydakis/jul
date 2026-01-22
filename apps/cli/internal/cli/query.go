package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lydakis/jul/cli/internal/client"
	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/notes"
	"github.com/lydakis/jul/cli/internal/output"
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

			results, err := localQuery(client.QueryFilters{
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

			output.RenderQuery(os.Stdout, results, output.DefaultOptions())
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

func localQuery(filters client.QueryFilters) ([]client.QueryResult, error) {
	checkpoints, err := listCheckpoints()
	if err != nil {
		return nil, err
	}
	results := make([]client.QueryResult, 0, len(checkpoints))
	limit := filters.Limit
	if limit <= 0 {
		limit = 20
	}

	for _, cp := range checkpoints {
		message := strings.TrimSpace(cp.Message)
		changeID := cp.ChangeID
		if changeID == "" {
			changeID = gitutil.FallbackChangeID(cp.SHA)
		}

		var att client.Attestation
		found, err := notes.ReadJSON(notes.RefAttestationsCheckpoint, cp.SHA, &att)
		if err != nil {
			return nil, err
		}

		res := client.QueryResult{
			CommitSHA: cp.SHA,
			ChangeID:  changeID,
			Author:    cp.Author,
			Message:   message,
			CreatedAt: cp.When,
		}
		if found {
			res.AttestationStatus = att.Status
			res.TestStatus = att.TestStatus
			res.CompileStatus = att.CompileStatus
			res.CoverageLinePct = att.CoverageLinePct
			res.CoverageBranchPct = att.CoverageBranchPct
		}

		if !matchQueryFilters(res, att, found, filters) {
			continue
		}

		results = append(results, res)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func matchQueryFilters(res client.QueryResult, att client.Attestation, hasAtt bool, filters client.QueryFilters) bool {
	if filters.Author != "" {
		author := strings.ToLower(res.Author)
		want := strings.ToLower(strings.TrimSpace(filters.Author))
		if !strings.Contains(author, want) {
			return false
		}
	}
	if filters.Since != nil && res.CreatedAt.Before(*filters.Since) {
		return false
	}
	if filters.Until != nil && res.CreatedAt.After(*filters.Until) {
		return false
	}
	if filters.ChangeID != "" && res.ChangeID != filters.ChangeID {
		return false
	}
	if filters.Tests != "" {
		if !hasAtt {
			return false
		}
		status := att.TestStatus
		if status == "" {
			status = att.Status
		}
		if status != filters.Tests {
			return false
		}
	}
	if filters.Compiles != nil {
		if !hasAtt {
			return false
		}
		status := att.CompileStatus
		if status == "" {
			status = att.Status
		}
		want := "fail"
		if *filters.Compiles {
			want = "pass"
		}
		if status != want {
			return false
		}
	}
	if filters.CoverageMin != nil {
		if !hasAtt || att.CoverageLinePct == nil || *att.CoverageLinePct < *filters.CoverageMin {
			return false
		}
	}
	if filters.CoverageMax != nil {
		if !hasAtt || att.CoverageLinePct == nil || *att.CoverageLinePct > *filters.CoverageMax {
			return false
		}
	}
	return true
}
