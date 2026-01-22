package agent

import "testing"

func TestApplyPromptTemplateReplaces(t *testing.T) {
	args := []string{"run", "--prompt", "$PROMPT", "--file", "$ATTACHMENT"}
	prompt := "Review this"
	attachment := "/tmp/review.txt"

	out, replaced := applyPromptTemplate(args, prompt, attachment)
	if !replaced {
		t.Fatalf("expected replacement")
	}
	if out[2] != prompt {
		t.Fatalf("expected prompt replacement, got %q", out[2])
	}
	if out[4] != attachment {
		t.Fatalf("expected attachment replacement, got %q", out[4])
	}
}

func TestApplyPromptTemplateNoReplacement(t *testing.T) {
	args := []string{"run", "--mode", "json"}
	out, replaced := applyPromptTemplate(args, "prompt", "/tmp/file")
	if replaced {
		t.Fatalf("did not expect replacement")
	}
	if len(out) != len(args) {
		t.Fatalf("expected output length %d, got %d", len(args), len(out))
	}
}
