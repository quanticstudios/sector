package view

import (
	"testing"
	"time"
)

func TestNewSamplesEveryTwoSeconds(t *testing.T) {
	if got := New(nil).Interval(); got != 2*time.Second {
		t.Fatalf("default interval %v, want 2s", got)
	}
}
