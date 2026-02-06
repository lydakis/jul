package cli

import (
	"strings"
	"testing"
	"time"
)

func TestShouldAutoSyncSkipsSyncHeavyCommands(t *testing.T) {
	commands := []string{"sync", "checkpoint", "apply", "ws", "draft"}
	for _, command := range commands {
		if shouldAutoSync(command) {
			t.Fatalf("expected %q to skip auto sync", command)
		}
		if shouldAutoSync(strings.ToUpper(command)) {
			t.Fatalf("expected %q to skip auto sync regardless of case", command)
		}
	}
}

func TestShouldAutoSyncAllowsReadCommands(t *testing.T) {
	commands := []string{"status", "log", "diff", "show"}
	for _, command := range commands {
		if !shouldAutoSync(command) {
			t.Fatalf("expected %q to allow auto sync", command)
		}
	}
}

func TestFormatSyncElapsed(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "negative", in: -100 * time.Millisecond, want: "0.0s"},
		{name: "sub-second", in: 250 * time.Millisecond, want: "0.2s"},
		{name: "round-second", in: 1490 * time.Millisecond, want: "1s"},
		{name: "multi-second", in: 3200 * time.Millisecond, want: "3s"},
	}

	for _, tc := range cases {
		if got := formatSyncElapsed(tc.in); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}
