package policy

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPromotePolicyInlineComments(t *testing.T) {
	repoRoot := t.TempDir()
	policyDir := filepath.Join(repoRoot, ".jul")
	if err := os.MkdirAll(policyDir, 0o755); err != nil {
		t.Fatalf("mkdir policy dir: %v", err)
	}

	policy := `
# top-level comment
[promote]
strategy = "rebase"  # rebase | squash | merge
min_coverage_pct = 92.5 # coverage target
require_suggestions_addressed = false # warn only
required_checks = ["ci", "lint"] # checks list
`
	if err := os.WriteFile(filepath.Join(policyDir, "policy.toml"), []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy file: %v", err)
	}

	parsed, ok, err := LoadPromotePolicy(repoRoot, "")
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if !ok {
		t.Fatalf("expected policy to load")
	}
	if parsed.Strategy != "rebase" {
		t.Fatalf("expected strategy rebase, got %q", parsed.Strategy)
	}
	if parsed.MinCoveragePct == nil || math.Abs(*parsed.MinCoveragePct-92.5) > 0.0001 {
		t.Fatalf("expected min coverage 92.5, got %v", parsed.MinCoveragePct)
	}
	if parsed.RequireSuggestionsAddressed == nil || *parsed.RequireSuggestionsAddressed {
		t.Fatalf("expected require_suggestions_addressed false, got %v", parsed.RequireSuggestionsAddressed)
	}
	if len(parsed.RequiredChecks) != 2 || parsed.RequiredChecks[0] != "ci" || parsed.RequiredChecks[1] != "lint" {
		t.Fatalf("unexpected required checks: %#v", parsed.RequiredChecks)
	}
}
