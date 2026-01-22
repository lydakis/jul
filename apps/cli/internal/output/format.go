package output

import (
	"io"
	"os"
	"strings"
)

type Options struct {
	Emoji bool
	Color bool
}

func DefaultOptions() Options {
	opts := Options{
		Emoji: true,
		Color: true,
	}
	if envBool("JUL_NO_EMOJI") {
		opts.Emoji = false
	}
	if envBool("JUL_NO_COLOR") || envBool("NO_COLOR") {
		opts.Color = false
	}
	if envBool("JUL_COLOR") {
		opts.Color = true
	}
	if !isTerminal(os.Stdout) && !envBool("JUL_COLOR") {
		opts.Color = false
	}
	return opts
}

func writeKV(w io.Writer, label, value string, width int) {
	if value == "" {
		return
	}
	if width <= 0 {
		width = len(label)
	}
	padding := width - len(label)
	if padding < 0 {
		padding = 0
	}
	spaces := strings.Repeat(" ", padding)
	_, _ = io.WriteString(w, label)
	_, _ = io.WriteString(w, spaces)
	_, _ = io.WriteString(w, ": ")
	_, _ = io.WriteString(w, value)
	_, _ = io.WriteString(w, "\n")
}

func writeKVIcon(w io.Writer, icon, label, value string, width int) {
	if value == "" {
		return
	}
	if icon == "" {
		writeKV(w, label, value, width)
		return
	}
	if width <= 0 {
		width = len(label)
	}
	padding := width - len(label)
	if padding < 0 {
		padding = 0
	}
	spaces := strings.Repeat(" ", padding)
	_, _ = io.WriteString(w, icon)
	_, _ = io.WriteString(w, label)
	_, _ = io.WriteString(w, spaces)
	_, _ = io.WriteString(w, ": ")
	_, _ = io.WriteString(w, value)
	_, _ = io.WriteString(w, "\n")
}

func statusIcon(status string, opts Options) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success":
		if opts.Emoji {
			return "✓ "
		}
		return "OK "
	case "fail", "failed", "error":
		if opts.Emoji {
			return "✗ "
		}
		return "X "
	case "running", "in_progress":
		if opts.Emoji {
			return "… "
		}
		return "... "
	case "pending", "queued":
		if opts.Emoji {
			return "• "
		}
		return ". "
	case "warning", "warn":
		if opts.Emoji {
			return "⚠ "
		}
		return "! "
	default:
		return ""
	}
}

func linePrefix(opts Options) string {
	if opts.Emoji {
		return "• "
	}
	return "- "
}

func statusText(status string, opts Options) string {
	text := strings.ToLower(strings.TrimSpace(status))
	if text == "" {
		return ""
	}
	if !opts.Color {
		return text
	}
	return colorize(text, statusColor(text))
}

func statusIconColored(status string, opts Options) string {
	icon := statusIcon(status, opts)
	if icon == "" || !opts.Color {
		return icon
	}
	return colorize(icon, statusColor(status))
}

func statusColor(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success":
		return ansiGreen
	case "fail", "failed", "error":
		return ansiRed
	case "running", "in_progress":
		return ansiCyan
	case "pending", "queued":
		return ansiBlue
	case "warning", "warn", "stale":
		return ansiYellow
	default:
		return ansiGray
	}
}

func colorize(text, color string) string {
	if color == "" {
		return text
	}
	return color + text + ansiReset
}

func envBool(key string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiGray   = "\x1b[90m"
)
