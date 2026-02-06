package syncer

import (
	"testing"

	"github.com/lydakis/jul/cli/internal/config"
)

func TestWorkspaceIDHasExplicitUser(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{input: "user/@", want: true},
		{input: "@/@", want: true},
		{input: "@", want: false},
		{input: "/@", want: false},
		{input: " ", want: false},
	}

	for _, tc := range cases {
		if got := workspaceIDHasExplicitUser(tc.input); got != tc.want {
			t.Fatalf("workspaceIDHasExplicitUser(%q)=%v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestWorkspaceNeedsNamespaceResolution(t *testing.T) {
	t.Setenv(config.EnvWorkspace, "user/@")
	if workspaceNeedsNamespaceResolution() {
		t.Fatalf("expected no namespace resolution when workspace env has explicit user")
	}

	t.Setenv(config.EnvWorkspace, "@")
	if !workspaceNeedsNamespaceResolution() {
		t.Fatalf("expected namespace resolution when workspace env omits user")
	}
}
