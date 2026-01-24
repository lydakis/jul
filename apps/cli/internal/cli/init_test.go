package cli

import (
	"reflect"
	"testing"
)

func TestNormalizeInitArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "flags-first",
			in:   []string{"--server", "/tmp/repos", "--create-remote", "demo"},
			want: []string{"--server", "/tmp/repos", "--create-remote", "demo"},
		},
		{
			name: "positional-first",
			in:   []string{"demo", "--server", "/tmp/repos", "--create-remote"},
			want: []string{"--server", "/tmp/repos", "--create-remote", "demo"},
		},
		{
			name: "equals-flag",
			in:   []string{"demo", "--remote=origin", "--no-hooks"},
			want: []string{"--remote=origin", "--no-hooks", "demo"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeInitArgs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected args: got %v, want %v", got, tc.want)
			}
		})
	}
}
