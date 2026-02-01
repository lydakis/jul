package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
)

func newLogCommand() Command {
	return Command{
		Name:    "log",
		Summary: "Show checkpoint history",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("log", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			limit := fs.Int("limit", 20, "Max checkpoints to show")
			changeID := fs.String("change-id", "", "Filter by Change-Id")
			showTraces := fs.Bool("traces", false, "Include trace history")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			entries, err := listCheckpoints()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to list checkpoints: %v\n", err)
				return 1
			}

			filtered := make([]output.LogEntry, 0, len(entries))
			for _, cp := range entries {
				if *changeID != "" && cp.ChangeID != *changeID {
					continue
				}
				attView, _ := resolveAttestationView(cp.SHA)
				suggestions, _ := metadata.ListSuggestions(cp.ChangeID, "pending", 1000)
				entry := output.LogEntry{
					CommitSHA:   cp.SHA,
					ChangeID:    cp.ChangeID,
					Author:      cp.Author,
					Message:     firstLine(cp.Message),
					When:        cp.When.Format("2006-01-02 15:04:05"),
					Suggestions: len(suggestions),
				}
				if *showTraces {
					entry.Traces = traceSummaries(cp.Message)
				}
				if attView.Status != "" {
					entry.AttestationStatus = attView.Status
					entry.AttestationStale = attView.Stale
					entry.AttestationInheritedFrom = attView.InheritedFrom
				}
				filtered = append(filtered, entry)
				if *limit > 0 && len(filtered) >= *limit {
					break
				}
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(filtered); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			output.RenderLog(os.Stdout, filtered, output.DefaultOptions())
			return 0
		},
	}
}

func traceSummaries(message string) []output.TraceSummary {
	head := strings.TrimSpace(gitutil.ExtractTraceHead(message))
	if head == "" {
		return nil
	}
	base := strings.TrimSpace(gitutil.ExtractTraceBase(message))
	chain, err := traceChain(base, head)
	if err != nil || len(chain) == 0 {
		chain = []string{head}
	}
	traces := make([]output.TraceSummary, 0, len(chain))
	for _, sha := range chain {
		sha = strings.TrimSpace(sha)
		if sha == "" {
			continue
		}
		note, _ := metadata.GetTrace(sha)
		if note != nil && (note.TraceType == "merge" || note.TraceType == "restack") {
			continue
		}
		trace := output.TraceSummary{TraceSHA: sha}
		if note != nil {
			trace.TraceType = note.TraceType
			trace.Agent = note.Agent
			trace.PromptSummary = note.PromptSummary
		}
		traces = append(traces, trace)
	}
	return traces
}

func traceChain(baseSHA, headSHA string) ([]string, error) {
	if strings.TrimSpace(headSHA) == "" {
		return nil, nil
	}
	var revRange string
	if strings.TrimSpace(baseSHA) != "" {
		revRange = fmt.Sprintf("%s..%s", baseSHA, headSHA)
	} else {
		revRange = headSHA
	}
	out, err := gitutil.Git("rev-list", "--reverse", revRange)
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(out)
	if strings.TrimSpace(baseSHA) != "" {
		lines = append([]string{strings.TrimSpace(baseSHA)}, lines...)
	}
	return lines, nil
}
