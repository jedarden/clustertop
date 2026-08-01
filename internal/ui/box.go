package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jedarden/clustertop/internal/fetch"
)

// Box sizing: content width is stretched to fill whatever terminal width is
// available (not fixed, not sized to each node's own longest field) so every
// box in the render shares one width — see
// docs/notes/node-box-grid-mockup.md open question 2.
const (
	boxMinContent = 16 // "pool: memory1-30" is 17 runes; below this a box stops being readable
	boxMaxContent = 30 // stop stretching once a box would mostly be whitespace
	boxChrome     = 4  // 2 border runes + 1 padding rune each side
	boxGap        = 1  // blank column between adjacent boxes in a row
	clusterChrome = 4  // outer cluster border: 2 border runes + 1 padding rune each side
)

// truncate shortens s to at most width runes, appending an ellipsis when it
// had to cut. Rune-based (not byte-based) so multi-byte glyphs in warnings
// never split mid-character.
func truncate(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 1 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "…"
}

// gridLayout picks how many node-boxes fit per row and how wide each box's
// content area should be for a cluster section of the given width. Columns
// are chosen against the *minimum* readable box width; leftover space is
// then distributed evenly back into the boxes (capped at boxMaxContent)
// rather than left as dead space at the row's right edge.
func gridLayout(sectionWidth int) (cols, contentWidth int) {
	minTotal := boxMinContent + boxChrome
	avail := sectionWidth
	if avail < minTotal {
		return 1, boxMinContent
	}
	cols = (avail + boxGap) / (minTotal + boxGap)
	if cols < 1 {
		cols = 1
	}
	totalBoxWidth := (avail - (cols-1)*boxGap) / cols
	contentWidth = totalBoxWidth - boxChrome
	if contentWidth > boxMaxContent {
		contentWidth = boxMaxContent
	}
	if contentWidth < boxMinContent {
		contentWidth = boxMinContent
	}
	return cols, contentWidth
}

// statusLine renders "<glyph> <label>[ ⚠ <warning>]" colored by readiness,
// truncated to width. The warning suffix is truncated independently so a
// long warning message can never push the glyph/label off the front of the
// line.
func statusLine(n fetch.NodeRow, width int) string {
	symbol, label, style := "●", "Ready", styleReady
	if !n.Ready {
		symbol, label, style = "⬤", "NotReady", styleNotReady
	}
	base := symbol + " " + label
	if n.Warning == "" {
		return style.Render(truncate(base, width))
	}

	baseRunes := len([]rune(base))
	remaining := width - baseRunes
	if remaining <= 0 {
		return style.Render(truncate(base, width))
	}
	warnSuffix := truncate(" ⚠ "+n.Warning, remaining)
	return style.Render(base) + styleWarning.Render(warnSuffix)
}

// nodeBoxLines builds the fixed-order content lines of one node box, each
// pre-truncated to contentWidth. lipgloss right-pads shorter lines when the
// box style's Width is applied, so callers don't need to pad here.
func nodeBoxLines(n fetch.NodeRow, contentWidth int) []string {
	return []string{
		truncate(n.Name, contentWidth),
		statusLine(n, contentWidth),
		truncate("roles: "+n.Roles, contentWidth),
		truncate("pool: "+n.PoolType, contentWidth),
		truncate(n.Version, contentWidth),
		truncate("age: "+n.Age, contentWidth),
	}
}

// nodeBoxBorderColor picks a border color for one node's box — the same
// green/red split as its status line, so an unhealthy node is visible even
// when a box's text content scrolls out of view.
func nodeBoxBorderColor(n fetch.NodeRow) lipgloss.Color {
	if n.Ready {
		return colorReady
	}
	return colorNotReady
}

// renderNodeBox draws one bordered box for a single node. lipgloss's Width()
// sets the width of the padded content area, not the text area inside the
// padding — with Padding(0, 1) that means Width() must be contentWidth+2 for
// the text itself (already truncated to contentWidth) to land without an
// unwanted extra wrap. Confirmed empirically; see docs/notes/node-box-grid-mockup.md.
func renderNodeBox(n fetch.NodeRow, contentWidth int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(nodeBoxBorderColor(n)).
		Padding(0, 1).
		Width(contentWidth + 2)
	return style.Render(strings.Join(nodeBoxLines(n, contentWidth), "\n"))
}

// renderNodeGrid arranges every node's box into rows that wrap to fit
// sectionWidth, joining each row left-to-right and stacking rows top-to-
// bottom. This is the whole grid a cluster's border wraps around.
func renderNodeGrid(nodes []fetch.NodeRow, sectionWidth int) string {
	if len(nodes) == 0 {
		return ""
	}
	cols, contentWidth := gridLayout(sectionWidth)

	boxes := make([]string, len(nodes))
	for i, n := range nodes {
		boxes[i] = renderNodeBox(n, contentWidth)
	}

	gap := strings.Repeat(" ", boxGap)
	var rows []string
	for i := 0; i < len(boxes); i += cols {
		end := i + cols
		if end > len(boxes) {
			end = len(boxes)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, joinWithGap(boxes[i:end], gap)...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// joinWithGap interleaves gap between each element of boxes, without adding
// a trailing gap after the last one.
func joinWithGap(boxes []string, gap string) []string {
	if len(boxes) == 0 {
		return nil
	}
	out := make([]string, 0, len(boxes)*2-1)
	for i, b := range boxes {
		if i > 0 {
			out = append(out, gap)
		}
		out = append(out, b)
	}
	return out
}
