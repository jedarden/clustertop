package ui

import (
	"strings"
	"testing"

	"github.com/jedarden/clustertop/internal/fetch"
)

func TestTruncate_ShorterThanWidthUnchanged(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate(short) = %q, want unchanged", got)
	}
}

func TestTruncate_LongerThanWidthEllipsized(t *testing.T) {
	if got := truncate("way-too-long-name", 8); got != "way-too…" {
		t.Errorf("truncate long = %q, want %q", got, "way-too…")
	}
}

func TestStatusLine_ReadyContainsGlyphAndLabel(t *testing.T) {
	n := fetch.NodeRow{Name: "n", Ready: true}
	line := statusLine(n, 20)
	if !strings.Contains(line, "●") || !strings.Contains(line, "Ready") {
		t.Errorf("expected ready glyph+label in %q", line)
	}
}

func TestStatusLine_WarningMarkerPresent(t *testing.T) {
	n := fetch.NodeRow{Name: "n", Ready: false, Warning: "scale-down tainted"}
	line := statusLine(n, 40)
	for _, want := range []string{"⬤", "NotReady", "⚠", "scale-down tainted"} {
		if !strings.Contains(line, want) {
			t.Errorf("statusLine = %q, missing %q", line, want)
		}
	}
}

func TestStatusLine_LongWarningNeverPushesGlyphOut(t *testing.T) {
	n := fetch.NodeRow{Name: "n", Ready: false, Warning: strings.Repeat("x", 100)}
	line := statusLine(n, 20)
	if !strings.Contains(line, "⬤") || !strings.Contains(line, "NotReady") {
		t.Errorf("expected glyph+label preserved even when warning is truncated, got %q", line)
	}
}

func TestNodeBoxLines_ContentOrderAndValues(t *testing.T) {
	n := fetch.NodeRow{
		Name: "n1", Roles: "<none>", PoolType: "memory1-30",
		Version: "v1.33.0", Age: "45d", Ready: true,
	}
	lines := nodeBoxLines(n, 20)
	if len(lines) != 6 {
		t.Fatalf("expected 6 box lines, got %d", len(lines))
	}
	want := map[int]string{
		0: "n1",
		2: "roles: <none>",
		3: "pool: memory1-30",
		4: "v1.33.0",
		5: "age: 45d",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestGridLayout_WiderTerminalFitsMoreColumns(t *testing.T) {
	narrowCols, _ := gridLayout(30)
	wideCols, _ := gridLayout(200)
	if wideCols <= narrowCols {
		t.Errorf("expected more columns at width 200 (%d) than width 30 (%d)", wideCols, narrowCols)
	}
}

func TestGridLayout_NeverBelowMinContentWidth(t *testing.T) {
	_, content := gridLayout(5)
	if content < boxMinContent {
		t.Errorf("content width %d fell below boxMinContent %d", content, boxMinContent)
	}
}

func TestGridLayout_CapsAtMaxContentWidth(t *testing.T) {
	// A single column (avail=39 doesn't fit two boxes) with lots of leftover
	// width would otherwise stretch to 35 — must be capped at boxMaxContent.
	_, content := gridLayout(39)
	if content != boxMaxContent {
		t.Errorf("expected content width capped at %d, got %d", boxMaxContent, content)
	}
}

func TestRenderNodeGrid_EmptyInput(t *testing.T) {
	if got := renderNodeGrid(nil, 80); got != "" {
		t.Errorf("expected empty grid for no nodes, got %q", got)
	}
}
