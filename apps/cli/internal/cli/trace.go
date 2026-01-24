package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lydakis/jul/cli/internal/ci"
	"github.com/lydakis/jul/cli/internal/config"
	"github.com/lydakis/jul/cli/internal/metadata"
	"github.com/lydakis/jul/cli/internal/output"
	"github.com/lydakis/jul/cli/internal/syncer"
)

func newTraceCommand() Command {
	return Command{
		Name:    "trace",
		Summary: "Record a trace for the current working tree",
		Run: func(args []string) int {
			fs := flag.NewFlagSet("trace", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			prompt := fs.String("prompt", "", "Prompt text to attach")
			promptStdin := fs.Bool("prompt-stdin", false, "Read prompt from stdin")
			agent := fs.String("agent", "", "Agent name")
			sessionID := fs.String("session-id", "", "Session identifier")
			turn := fs.Int("turn", 0, "Turn number within session")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			var promptText string
			if *promptStdin {
				if strings.TrimSpace(*prompt) != "" {
					fmt.Fprintln(os.Stderr, "cannot combine --prompt with --prompt-stdin")
					return 1
				}
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to read prompt: %v\n", err)
					return 1
				}
				promptText = strings.TrimSpace(string(data))
			} else {
				promptText = strings.TrimSpace(*prompt)
			}

			res, err := syncer.Trace(syncer.TraceOptions{
				Prompt:          promptText,
				Agent:           strings.TrimSpace(*agent),
				SessionID:       strings.TrimSpace(*sessionID),
				Turn:            *turn,
				Force:           true,
				UpdateCanonical: true,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "trace failed: %v\n", err)
				return 1
			}

			if *jsonOut {
				payload := struct {
					TraceSHA     string `json:"trace_sha"`
					TraceRef     string `json:"trace_ref"`
					TraceSyncRef string `json:"trace_sync_ref"`
					CanonicalSHA string `json:"canonical_sha,omitempty"`
					PromptHash   string `json:"prompt_hash,omitempty"`
					Merged       bool   `json:"merged"`
					Skipped      bool   `json:"skipped"`
				}{
					TraceSHA:     res.TraceSHA,
					TraceRef:     res.TraceRef,
					TraceSyncRef: res.TraceSyncRef,
					CanonicalSHA: res.CanonicalSHA,
					PromptHash:   res.PromptHash,
					Merged:       res.Merged,
					Skipped:      res.Skipped,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(payload); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			label := "Trace created"
			if res.Skipped {
				label = "Trace unchanged"
			}
			fmt.Fprintf(os.Stdout, "%s (sha:%s).\n", label, shortSHA(res.TraceSHA))
			if strings.TrimSpace(*agent) != "" {
				fmt.Fprintf(os.Stdout, "  Agent: %s\n", strings.TrimSpace(*agent))
			}
			if promptText != "" {
				if config.TraceSyncPromptHash() && res.PromptHash != "" {
					fmt.Fprintf(os.Stdout, "  Prompt: [hash %s]\n", res.PromptHash)
				} else {
					fmt.Fprintln(os.Stdout, "  Prompt: [local only]")
				}
			}
			if res.Merged && res.CanonicalSHA != "" && res.CanonicalSHA != res.TraceSHA {
				fmt.Fprintf(os.Stdout, "  Trace tip merged: %s\n", shortSHA(res.CanonicalSHA))
			}
			if config.TraceRunOnTrace() && !res.Skipped {
				if att, err := metadata.GetTraceAttestation(res.TraceSHA); err == nil && att != nil && att.SignalsJSON != "" {
					var result ci.Result
					if err := json.Unmarshal([]byte(att.SignalsJSON), &result); err == nil {
						output.RenderCIResult(os.Stdout, result, output.DefaultOptions())
					}
				}
			}
			return 0
		},
	}
}

func shortSHA(sha string) string {
	trimmed := strings.TrimSpace(sha)
	if len(trimmed) <= 7 {
		return trimmed
	}
	return trimmed[:7]
}
