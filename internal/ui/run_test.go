package ui

import (
	"testing"
	"time"
)

func TestDefaultTimingBudget(t *testing.T) {
	if defaultRefreshEvery != 15*time.Second {
		t.Fatalf("defaultRefreshEvery = %v, want 15s", defaultRefreshEvery)
	}
	if defaultFetchTimeout != 10*time.Second {
		t.Fatalf("defaultFetchTimeout = %v, want 10s", defaultFetchTimeout)
	}
	if defaultFetchTimeout >= defaultRefreshEvery {
		t.Fatalf("defaultFetchTimeout = %v must remain below refresh interval %v", defaultFetchTimeout, defaultRefreshEvery)
	}
}
