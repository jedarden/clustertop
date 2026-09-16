package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jedarden/clustertop/internal/fetch"
)

// The escape sequences lipgloss emits for the style.go palette under the
// ANSI256 profile — matched literally so color assertions can't drift from
// the palette without an obvious test failure.
const (
	ansiGreen  = "\x1b[38;5;42m"  // colorReady
	ansiRed    = "\x1b[38;5;196m" // colorNotReady / colorUnreachable
	ansiYellow = "\x1b[38;5;214m" // colorWarning
	ansiGray   = "\x1b[38;5;244m" // colorPending
)

// setANSI256 forces color on for tests that assert on escape sequences. `go
// test` attaches stdout to a pipe, where lipgloss would otherwise detect the
// Ascii profile and strip every color — and the point of these tests is that
// the right color is emitted at all.
func setANSI256(t *testing.T) {
	t.Helper()
	old := lipgloss.DefaultRenderer().ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

// setAscii forces color off for structural (width/line-count) assertions, so
// they stay deterministic even if the surrounding environment ever renders
// with a color profile attached.
func setAscii(t *testing.T) {
	t.Helper()
	old := lipgloss.DefaultRenderer().ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

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

// gridOfNodes builds n ready nodes with distinct names, for layout tests that
// don't care about node content.
func gridOfNodes(n int) []fetch.NodeRow {
	nodes := make([]fetch.NodeRow, n)
	for i := range nodes {
		nodes[i] = fetch.NodeRow{Name: fmt.Sprintf("node-%d", i), Ready: true}
	}
	return nodes
}

func TestRenderNodeGrid_WrapsIntoRowsOfFittingBoxes(t *testing.T) {
	setAscii(t)

	// At section width 41, gridLayout fits exactly two 20-wide boxes per row.
	// Pinned here so the row arithmetic below can't silently drift.
	cols, contentWidth := gridLayout(41)
	if cols != 2 || contentWidth != 16 {
		t.Fatalf("gridLayout(41) = %d cols, content %d; test assumes 2 cols of content width 16", cols, contentWidth)
	}

	rowHeight := len(strings.Split(renderNodeBox(gridOfNodes(1)[0], contentWidth), "\n"))
	out := renderNodeGrid(gridOfNodes(5), 41)
	lines := strings.Split(out, "\n")

	// 5 nodes at 2 per row = 2 full rows + 1 partial row.
	if len(lines) != 3*rowHeight {
		t.Fatalf("5 nodes at 2 cols should stack 3 rows of %d lines, got %d:\n%s", rowHeight, len(lines), out)
	}
	for row, wantBoxes := range []int{2, 2, 1} {
		start := row * rowHeight
		if got := strings.Count(lines[start], "╭"); got != wantBoxes {
			t.Errorf("row %d starts with %d boxes, want %d (line %q)", row, got, wantBoxes, lines[start])
		}
	}
}

// A partial last row must not leave the grid ragged: JoinVertical pads the
// short row out to the same width the full rows reached.
func TestRenderNodeGrid_PartialLastRowStaysFullWidth(t *testing.T) {
	setAscii(t)
	out := renderNodeGrid(gridOfNodes(5), 41) // 2+2+1 rows at width 41
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != 41 {
			t.Errorf("line %d rendered at width %d, want 41: %q", i, got, line)
		}
	}
}

// Box width comes from the grid's shared contentWidth, never from a box's own
// content — one node with a 60-rune everything must render exactly as wide as
// a minimal one.
func TestRenderNodeBox_WidthIgnoresContentLength(t *testing.T) {
	setAscii(t)
	short := fetch.NodeRow{Name: "n", Ready: true}
	long := fetch.NodeRow{
		Name:     strings.Repeat("x", 60),
		Roles:    strings.Repeat("r", 60),
		PoolType: strings.Repeat("p", 60),
		Version:  strings.Repeat("v", 60),
		Age:      strings.Repeat("a", 60),
		Ready:    true,
		Warning:  strings.Repeat("w", 60),
	}
	for _, tc := range []struct {
		label string
		node  fetch.NodeRow
	}{
		{"minimal content", short},
		{"60-rune content", long},
	} {
		lines := strings.Split(renderNodeBox(tc.node, 20), "\n")
		for i, line := range lines {
			if got := lipgloss.Width(line); got != 24 { // 20 content+padding, +2 border runes
				t.Errorf("%s: line %d width %d, want 24 (no wrap allowed): %q", tc.label, i, got, line)
			}
		}
	}
}

func TestRenderNodeBox_LongNameTruncatedNotWrapped(t *testing.T) {
	setAscii(t)
	const contentWidth = 16
	name := "prod-instance-17825485895130660" // 31 runes, from a real cluster
	box := renderNodeBox(fetch.NodeRow{Name: name, Ready: true}, contentWidth)

	lines := strings.Split(box, "\n")
	if len(lines) != 8 {
		t.Fatalf("a too-long name must be truncated, not wrapped into an extra line; got %d lines:\n%s", len(lines), box)
	}
	nameLine := strings.Trim(lines[1], "│ ")
	want := truncate(name, contentWidth)
	if nameLine != want {
		t.Errorf("name line = %q, want truncated-to-%d %q", nameLine, contentWidth, want)
	}
	if !strings.HasSuffix(want, "…") {
		t.Errorf("expected the truncated name %q to end with an ellipsis", want)
	}
}

func TestNodeBoxBorderColor_FollowsReadiness(t *testing.T) {
	cases := []struct {
		name string
		node fetch.NodeRow
		want lipgloss.Color
	}{
		{"ready", fetch.NodeRow{Ready: true}, colorReady},
		{"not ready", fetch.NodeRow{Ready: false}, colorNotReady},
		{"warned but ready", fetch.NodeRow{Ready: true, Warning: "on-demand pool"}, colorReady},
	}
	for _, tc := range cases {
		if got := nodeBoxBorderColor(tc.node); got != tc.want {
			t.Errorf("%s: border color %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRenderNodeBox_BorderColorReflectsReadiness(t *testing.T) {
	setANSI256(t)
	ready := renderNodeBox(fetch.NodeRow{Name: "n", Ready: true}, 16)
	notReady := renderNodeBox(fetch.NodeRow{Name: "n", Ready: false}, 16)

	if !strings.Contains(ready, ansiGreen) {
		t.Errorf("ready node's box border should be green, got %q", ready)
	}
	if strings.Contains(ready, ansiRed) {
		t.Errorf("ready node's box should carry no red, got %q", ready)
	}
	if !strings.Contains(notReady, ansiRed) {
		t.Errorf("not-ready node's box border should be red, got %q", notReady)
	}
}

func TestStatusLine_ColorsFollowReadinessAndWarning(t *testing.T) {
	setANSI256(t)

	ready := statusLine(fetch.NodeRow{Ready: true}, 40)
	if !strings.Contains(ready, ansiGreen) || strings.Contains(ready, ansiRed) {
		t.Errorf("ready status line should be green-only, got %q", ready)
	}

	// The warning suffix is yellow while the base stays red — one style on one
	// line, not one style for the whole line.
	degraded := statusLine(fetch.NodeRow{Ready: false, Warning: "scale-down tainted"}, 40)
	if !strings.Contains(degraded, ansiRed) {
		t.Errorf("not-ready status line base should be red, got %q", degraded)
	}
	if !strings.Contains(degraded, ansiYellow) {
		t.Errorf("warning suffix should be yellow, got %q", degraded)
	}
}
