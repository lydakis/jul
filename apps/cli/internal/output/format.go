package output

import (
	"io"
	"strings"
)

type Options struct {
	Emoji bool
}

func DefaultOptions() Options {
	return Options{Emoji: true}
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

func statusIcon(status string, opts Options) string {
	if !opts.Emoji {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success":
		return "✓ "
	case "fail", "failed", "error":
		return "✗ "
	case "running", "in_progress":
		return "… "
	case "pending", "queued":
		return "• "
	case "warning", "warn":
		return "⚠ "
	default:
		return ""
	}
}
