package gitutil

import "testing"

func TestExtractTraceTrailers(t *testing.T) {
	msg := "feat: add auth\n\nChange-Id: Iabc\nTrace-Base: base123\nTrace-Head: head456\n"
	if got := ExtractTraceBase(msg); got != "base123" {
		t.Fatalf("expected base123, got %q", got)
	}
	if got := ExtractTraceHead(msg); got != "head456" {
		t.Fatalf("expected head456, got %q", got)
	}
}
