package cli

import "testing"

func TestSanitizeGeneratedCheckpointMessageStripsReservedTrailers(t *testing.T) {
	message := "feat: generated checkpoint message\n\nBody line\n\nChange-Id: I1111111111111111111111111111111111111111\nTrace-Head: deadbeef\nTrace-Base: badbase\n"
	got := sanitizeGeneratedCheckpointMessage(message)
	want := "feat: generated checkpoint message\n\nBody line"
	if got != want {
		t.Fatalf("expected sanitized message %q, got %q", want, got)
	}
}

func TestSanitizeGeneratedCheckpointMessageStripsChangeIDSpaceVariant(t *testing.T) {
	message := "feat: generated checkpoint message\n\nchange-id I2222222222222222222222222222222222222222\n"
	got := sanitizeGeneratedCheckpointMessage(message)
	want := "feat: generated checkpoint message"
	if got != want {
		t.Fatalf("expected sanitized message %q, got %q", want, got)
	}
}

func TestSanitizeGeneratedCheckpointMessagePreservesNonReservedLines(t *testing.T) {
	message := "feat: generated checkpoint message\n\nCo-authored-by: Test User <test@example.com>\n"
	got := sanitizeGeneratedCheckpointMessage(message)
	want := "feat: generated checkpoint message\n\nCo-authored-by: Test User <test@example.com>"
	if got != want {
		t.Fatalf("expected message to be preserved, got %q", got)
	}
}
