package integration

import (
	"testing"
	"time"
)

func TestPerfRatioExceededSkipsLowMedianJitter(t *testing.T) {
	exceeded := perfRatioExceeded(53*time.Millisecond, 183*time.Millisecond, 3.0)
	if exceeded {
		t.Fatalf("expected ratio check to be skipped for p50 below jitter floor")
	}
}

func TestPerfRatioExceededTriggersForStableMedian(t *testing.T) {
	exceeded := perfRatioExceeded(100*time.Millisecond, 350*time.Millisecond, 3.0)
	if !exceeded {
		t.Fatalf("expected ratio check to fail for p95/p50 ratio above threshold")
	}
}

func TestPerfRatioExceededPassesWithinLimit(t *testing.T) {
	exceeded := perfRatioExceeded(100*time.Millisecond, 300*time.Millisecond, 3.0)
	if exceeded {
		t.Fatalf("expected ratio check to pass when p95/p50 is within threshold")
	}
}
