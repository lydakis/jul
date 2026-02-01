package cli

import (
	"encoding/json"
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

type traceOutput struct {
	TraceSHA       string     `json:"trace_sha"`
	TraceRef       string     `json:"trace_ref,omitempty"`
	TraceSyncRef   string     `json:"trace_sync_ref,omitempty"`
	CanonicalSHA   string     `json:"canonical_sha,omitempty"`
	PromptHash     string     `json:"prompt_hash,omitempty"`
	PromptProvided bool       `json:"prompt_provided,omitempty"`
	PromptStored   bool       `json:"prompt_stored,omitempty"`
	Agent          string     `json:"agent,omitempty"`
	Merged         bool       `json:"merged"`
	Skipped        bool       `json:"skipped"`
	CI             *ci.Result `json:"ci,omitempty"`
}

func newTraceCommand() Command {
	return Command{
		Name:    "trace",
		Summary: "Record a trace for the current working tree",
		Run: func(args []string) int {
			fs, jsonOut := newFlagSet("trace")
			prompt := fs.String("prompt", "", "Prompt text to attach")
			promptStdin := fs.Bool("prompt-stdin", false, "Read prompt from stdin")
			agent := fs.String("agent", "", "Agent name")
			sessionID := fs.String("session-id", "", "Session identifier")
			turn := fs.Int("turn", 0, "Turn number within session")
			_ = fs.Parse(args)

			var promptText string
			if *promptStdin {
				if strings.TrimSpace(*prompt) != "" {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "trace_prompt_conflict", "cannot combine --prompt with --prompt-stdin", nil)
					} else {
						fmt.Fprintln(os.Stderr, "cannot combine --prompt with --prompt-stdin")
					}
					return 1
				}
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					if *jsonOut {
						_ = output.EncodeError(os.Stdout, "trace_prompt_read_failed", fmt.Sprintf("failed to read prompt: %v", err), nil)
					} else {
						fmt.Fprintf(os.Stderr, "failed to read prompt: %v\n", err)
					}
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
				if *jsonOut {
					_ = output.EncodeError(os.Stdout, "trace_failed", fmt.Sprintf("trace failed: %v", err), nil)
				} else {
					fmt.Fprintf(os.Stderr, "trace failed: %v\n", err)
				}
				return 1
			}

			out := traceOutput{
				TraceSHA:       res.TraceSHA,
				TraceRef:       res.TraceRef,
				TraceSyncRef:   res.TraceSyncRef,
				CanonicalSHA:   res.CanonicalSHA,
				PromptHash:     res.PromptHash,
				PromptProvided: promptText != "",
				PromptStored:   config.TraceSyncPromptHash() && res.PromptHash != "",
				Agent:          strings.TrimSpace(*agent),
				Merged:         res.Merged,
				Skipped:        res.Skipped,
			}
			if config.TraceRunOnTrace() && !res.Skipped {
				if att, err := metadata.GetTraceAttestation(res.TraceSHA); err == nil && att != nil && att.SignalsJSON != "" {
					var result ci.Result
					if err := json.Unmarshal([]byte(att.SignalsJSON), &result); err == nil {
						out.CI = &result
					}
				}
			}

			if *jsonOut {
				return writeJSON(out)
			}

			renderTraceOutput(out)
			return 0
		},
	}
}

func renderTraceOutput(out traceOutput) {
	label := "Trace created"
	if out.Skipped {
		label = "Trace unchanged"
	}
	fmt.Fprintf(os.Stdout, "%s (sha:%s).\n", label, shortSHA(out.TraceSHA))
	if strings.TrimSpace(out.Agent) != "" {
		fmt.Fprintf(os.Stdout, "  Agent: %s\n", strings.TrimSpace(out.Agent))
	}
	if out.PromptProvided {
		if out.PromptStored && out.PromptHash != "" {
			fmt.Fprintf(os.Stdout, "  Prompt: [hash %s]\n", out.PromptHash)
		} else {
			fmt.Fprintln(os.Stdout, "  Prompt: [local only]")
		}
	}
	if out.Merged && out.CanonicalSHA != "" && out.CanonicalSHA != out.TraceSHA {
		fmt.Fprintf(os.Stdout, "  Trace tip merged: %s\n", shortSHA(out.CanonicalSHA))
	}
	if out.CI != nil {
		output.RenderCIResult(os.Stdout, *out.CI, output.DefaultOptions())
	}
}

func shortSHA(sha string) string {
	trimmed := strings.TrimSpace(sha)
	if len(trimmed) <= 7 {
		return trimmed
	}
	return trimmed[:7]
}
