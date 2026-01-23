package syncer

import "testing"

func TestScrubSecrets(t *testing.T) {
	inputs := []string{
		"Bearer abc123token",
		"api_key=secretvalue",
		"token: supersecret",
		"sk-1234567890abcdef",
		"ghp_1234567890abcdef1234567890abcdef",
	}
	for _, input := range inputs {
		out := scrubSecrets(input)
		if out == input {
			t.Fatalf("expected scrubbed output for %q", input)
		}
		if out == "" {
			t.Fatalf("expected non-empty scrubbed output")
		}
	}
}
