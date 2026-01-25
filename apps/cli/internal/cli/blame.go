package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lydakis/jul/cli/internal/gitutil"
	"github.com/lydakis/jul/cli/internal/metadata"
)

type blameLine struct {
	Line      int
	Content   string
	CommitSHA string
	Author    string
	Summary   string
}

func newBlameCommand() Command {
	return Command{
		Name:    "blame",
		Summary: "Show line-by-line provenance",
		Run: func(args []string) int {
			args = normalizeFlagArgs(args)
			fs := flag.NewFlagSet("blame", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			showPrompts := fs.Bool("prompts", false, "Show prompts/summaries for trace lines")
			localOnly := fs.Bool("local", false, "Include local prompt text")
			verbose := fs.Bool("verbose", false, "Show full context per line")
			noTrace := fs.Bool("no-trace", false, "Disable trace attribution")
			jsonOut := fs.Bool("json", false, "Output JSON")
			_ = fs.Parse(args)

			target := strings.TrimSpace(fs.Arg(0))
			if target == "" {
				fmt.Fprintln(os.Stderr, "file path required")
				return 1
			}
			path, start, end, err := parseFileRange(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid file range: %v\n", err)
				return 1
			}

			repoRoot, err := gitutil.RepoTopLevel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to locate repo: %v\n", err)
				return 1
			}
			absPath := filepath.Join(repoRoot, path)
			if _, err := os.Stat(absPath); err != nil {
				fmt.Fprintf(os.Stderr, "file not found: %s\n", path)
				return 1
			}

			checkpoint, _ := latestCheckpoint()
			baseSHA := "HEAD"
			changeID := ""
			if checkpoint != nil {
				baseSHA = checkpoint.SHA
				changeID = checkpoint.ChangeID
			}

			mainLines, err := blameFile(repoRoot, baseSHA, path, start, end)
			if err != nil {
				fmt.Fprintf(os.Stderr, "blame failed: %v\n", err)
				return 1
			}
			traceLines := map[int]blameLine{}
			traceTip := ""
			if !*noTrace {
				if checkpoint != nil && checkpoint.TraceHead != "" {
					traceTip = checkpoint.TraceHead
				} else {
					user, workspace := workspaceParts()
					if workspace == "" {
						workspace = "@"
					}
					traceRef := fmt.Sprintf("refs/jul/traces/%s/%s", user, workspace)
					if gitutil.RefExists(traceRef) {
						if sha, err := gitutil.ResolveRef(traceRef); err == nil {
							traceTip = strings.TrimSpace(sha)
						}
					}
				}
				if traceTip != "" {
					trace, err := blameFile(repoRoot, traceTip, path, start, end)
					if err == nil {
						traceCache := map[string]string{}
						for _, line := range trace {
							line.CommitSHA = resolveTraceAttribution(repoRoot, line.CommitSHA, traceCache)
							traceLines[line.Line] = line
						}
					}
				}
			}

			if *jsonOut {
				out := buildBlameJSON(path, changeID, baseSHA, mainLines, traceLines, *showPrompts || *verbose, *localOnly)
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode json: %v\n", err)
					return 1
				}
				return 0
			}

			renderBlameText(changeID, mainLines, traceLines, *showPrompts || *verbose, *localOnly, *verbose)
			return 0
		},
	}
}

func normalizeFlagArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, 1)
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
		} else {
			positional = append(positional, arg)
		}
	}
	return append(flags, positional...)
}

func renderBlameText(changeID string, mainLines []blameLine, traceLines map[int]blameLine, showPrompts, localOnly, verbose bool) {
	cache := map[string]*metadata.TraceNote{}
	for _, line := range mainLines {
		trace := traceLines[line.Line]
		traceSHA := strings.TrimSpace(trace.CommitSHA)
		if changeID == "" {
			changeID = gitutil.FallbackChangeID(line.CommitSHA)
		}
		label := changeID
		if traceSHA != "" {
			label = fmt.Sprintf("%s (sha:%s)", changeID, shortSHA(traceSHA))
		}
		fmt.Fprintf(os.Stdout, "%4d │ %-60s %s\n", line.Line, line.Content, label)
		if !showPrompts && !verbose {
			continue
		}
		if traceSHA == "" {
			if verbose {
				fmt.Fprintf(os.Stdout, "     │ No trace metadata\n")
			}
			continue
		}
		note := cache[traceSHA]
		if note == nil {
			note, _ = metadata.GetTrace(traceSHA)
			cache[traceSHA] = note
		}
		prompt, summary := tracePromptDetails(traceSHA, note, localOnly)
		if verbose {
			if note != nil && note.Agent != "" {
				fmt.Fprintf(os.Stdout, "     │ Agent: %s\n", note.Agent)
			}
			if note != nil && note.SessionID != "" {
				fmt.Fprintf(os.Stdout, "     │ Session: %s\n", note.SessionID)
			}
			if summary != "" {
				fmt.Fprintf(os.Stdout, "     │ Summary: %s\n", summary)
			}
			if prompt != "" {
				fmt.Fprintf(os.Stdout, "     │ Prompt: %s\n", prompt)
			}
		} else if showPrompts {
			if summary != "" {
				fmt.Fprintf(os.Stdout, "     │ Summary: %s\n", summary)
			} else if note != nil && note.PromptHash != "" {
				fmt.Fprintf(os.Stdout, "     │ Prompt: %s\n", note.PromptHash)
			}
		}
	}
}

func buildBlameJSON(path, changeID, baseSHA string, mainLines []blameLine, traceLines map[int]blameLine, includePrompts, localOnly bool) map[string]any {
	type lineJSON struct {
		Line              int    `json:"line"`
		Content           string `json:"content"`
		CheckpointSHA     string `json:"checkpoint_sha"`
		CheckpointChange  string `json:"checkpoint_change_id"`
		TraceSHA          string `json:"trace_sha,omitempty"`
		Agent             string `json:"agent,omitempty"`
		PromptHash        string `json:"prompt_hash,omitempty"`
		PromptSummary     string `json:"prompt_summary,omitempty"`
		PromptFull        string `json:"prompt,omitempty"`
		SessionID         string `json:"session_id,omitempty"`
		Turn              int    `json:"turn,omitempty"`
		TraceSummaryLocal bool   `json:"trace_summary_local,omitempty"`
	}
	lines := make([]lineJSON, 0, len(mainLines))
	cache := map[string]*metadata.TraceNote{}
	for _, line := range mainLines {
		trace := traceLines[line.Line]
		traceSHA := strings.TrimSpace(trace.CommitSHA)
		entry := lineJSON{
			Line:             line.Line,
			Content:          line.Content,
			CheckpointSHA:    baseSHA,
			CheckpointChange: changeID,
			TraceSHA:         traceSHA,
		}
		if traceSHA != "" {
			note := cache[traceSHA]
			if note == nil {
				note, _ = metadata.GetTrace(traceSHA)
				cache[traceSHA] = note
			}
			if note != nil {
				entry.Agent = note.Agent
				entry.PromptHash = note.PromptHash
				entry.SessionID = note.SessionID
				entry.Turn = note.Turn
			}
			if includePrompts {
				prompt, summary := tracePromptDetails(traceSHA, note, localOnly)
				entry.PromptFull = prompt
				entry.PromptSummary = summary
				entry.TraceSummaryLocal = localOnly && summary != ""
			}
		}
		lines = append(lines, entry)
	}
	return map[string]any{
		"file":  path,
		"lines": lines,
	}
}

func tracePromptDetails(traceSHA string, note *metadata.TraceNote, localOnly bool) (string, string) {
	var prompt string
	var summary string
	if note != nil {
		summary = strings.TrimSpace(note.PromptSummary)
	}
	if localOnly {
		if localSummary, err := metadata.ReadTraceSummary(traceSHA); err == nil && localSummary != "" {
			summary = localSummary
		}
		if localPrompt, err := metadata.ReadTracePrompt(traceSHA); err == nil && localPrompt != "" {
			prompt = localPrompt
		}
	}
	return prompt, summary
}

func blameFile(repoRoot, ref, path string, start, end int) ([]blameLine, error) {
	args := []string{"-C", repoRoot, "blame", "--line-porcelain"}
	if start > 0 {
		if end < start {
			end = start
		}
		args = append(args, "-L", fmt.Sprintf("%d,%d", start, end))
	}
	args = append(args, ref, "--", path)
	out, err := gitutil.Git(args...)
	if err != nil {
		return nil, err
	}
	lines := parseBlamePorcelain(out)
	sort.Slice(lines, func(i, j int) bool { return lines[i].Line < lines[j].Line })
	return lines, nil
}

func parseBlamePorcelain(output string) []blameLine {
	lines := strings.Split(output, "\n")
	results := make([]blameLine, 0)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		sha := fields[0]
		lineNo, _ := strconv.Atoi(fields[2])
		entry := blameLine{CommitSHA: sha, Line: lineNo}
		for i+1 < len(lines) {
			i++
			value := lines[i]
			if strings.HasPrefix(value, "\t") {
				entry.Content = strings.TrimPrefix(value, "\t")
				results = append(results, entry)
				break
			}
			if strings.HasPrefix(value, "author ") {
				entry.Author = strings.TrimSpace(strings.TrimPrefix(value, "author "))
			}
			if strings.HasPrefix(value, "summary ") {
				entry.Summary = strings.TrimSpace(strings.TrimPrefix(value, "summary "))
			}
		}
	}
	return results
}

func resolveTraceAttribution(repoRoot, traceSHA string, cache map[string]string) string {
	sha := strings.TrimSpace(traceSHA)
	if sha == "" {
		return ""
	}
	if cached, ok := cache[sha]; ok {
		return cached
	}
	note, err := metadata.GetTrace(sha)
	if err == nil && note != nil {
		switch strings.TrimSpace(note.TraceType) {
		case "merge", "restack":
			for _, parent := range traceParents(repoRoot, sha) {
				attrib := resolveTraceAttribution(repoRoot, parent, cache)
				if strings.TrimSpace(attrib) != "" {
					cache[sha] = attrib
					return attrib
				}
			}
			cache[sha] = ""
			return ""
		}
	}
	cache[sha] = sha
	return sha
}

func traceParents(repoRoot, sha string) []string {
	out, err := gitutil.Git("-C", repoRoot, "rev-list", "--parents", "-n", "1", sha)
	if err != nil {
		return nil
	}
	fields := strings.Fields(out)
	if len(fields) <= 1 {
		return nil
	}
	return fields[1:]
}

func parseFileRange(arg string) (string, int, int, error) {
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) == 1 {
		return arg, 0, 0, nil
	}
	path := parts[0]
	rangePart := parts[1]
	if rangePart == "" {
		return path, 0, 0, nil
	}
	if strings.Contains(rangePart, "-") {
		segs := strings.SplitN(rangePart, "-", 2)
		start, err := strconv.Atoi(segs[0])
		if err != nil {
			return "", 0, 0, err
		}
		end, err := strconv.Atoi(segs[1])
		if err != nil {
			return "", 0, 0, err
		}
		return path, start, end, nil
	}
	start, err := strconv.Atoi(rangePart)
	if err != nil {
		return "", 0, 0, err
	}
	return path, start, start, nil
}
