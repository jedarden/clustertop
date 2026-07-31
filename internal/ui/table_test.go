package ui

import (
	"testing"

	"github.com/jedarden/clustertop/internal/fetch"
)

func TestToTableRows_DeterministicCells(t *testing.T) {
	nodes := []fetch.NodeRow{
		{
			Name: "memory1-30-a", Roles: "<none>", PoolType: "memory1-30",
			Version: "v1.33.0", Age: "45d", Ready: true,
		},
	}
	rows := toTableRows(nodes)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	want := []string{"memory1-30-a", "● Ready", "<none>", "memory1-30", "v1.33.0", "45d"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("cell %d = %q, want %q", i, got[i], w)
		}
	}
}

func TestToTableRows_WarningMarkerPresent(t *testing.T) {
	nodes := []fetch.NodeRow{
		{Name: "n", Ready: false, Warning: "scale-down tainted"},
	}
	rows := toTableRows(nodes)
	status := rows[0][1]
	if status != "⬤ NotReady ⚠ scale-down tainted" {
		t.Errorf("status cell = %q, missing/wrong warning marker", status)
	}
}

func TestToTableRows_EmptyInput(t *testing.T) {
	rows := toTableRows(nil)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows for nil input, got %d", len(rows))
	}
}
