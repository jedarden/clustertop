package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jedarden/clustertop/internal/fetch"
)

func (m Model) View() string {
	if m.Quitting {
		return ""
	}

	footer := styleFooter.Render("[q] quit  [r] refresh")

	if !m.ready {
		// Before the first WindowSizeMsg arrives, there's no known width to
		// size cluster borders/boxes to — fall back to the minimum grid.
		return renderBody(m.Clusters, 0) + "\n" + footer + "\n"
	}

	return m.Viewport.View() + "\n" + footer + "\n"
}

// renderBody renders every cluster section, without the top-level header or
// footer — this is what gets set as the viewport's scrollable content.
// width is the full terminal width; every cluster's border is sized to it so
// all cluster sections (and all node boxes within them) line up.
func renderBody(clusters []ClusterState, width int) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("clustertop — %d clusters", len(clusters))))
	b.WriteString("\n\n")
	for _, cs := range clusters {
		b.WriteString(renderClusterSection(cs, width))
		b.WriteString("\n\n")
	}
	return b.String()
}

// renderClusterSection draws one cluster's full bordered section: every node
// belonging to this cluster renders inside the same border, so the border
// itself is what communicates "these nodes are one cluster" (as opposed to
// the old per-cluster table, which had no enclosing border at all).
func renderClusterSection(cs ClusterState, width int) string {
	switch cs.Status {
	case StatusPending:
		title := styleHeader.Render(cs.Cluster.Name) + " " + styleStale.Render("— connecting…")
		return renderBorderedSection(title, colorPending, width, "")

	case StatusError:
		title := styleHeader.Render(cs.Cluster.Name) + " — " + styleUnreachable.Render("UNREACHABLE")
		staleness := ""
		if !cs.LastFetch.IsZero() {
			staleness = fmt.Sprintf(" (last seen %s ago)", time.Since(cs.LastFetch).Round(time.Second))
		}
		errText := ""
		if cs.Err != nil {
			errText = cs.Err.Error()
		}
		body := styleNotReady.Render(fmt.Sprintf("⚠ %s%s", errText, staleness))
		return renderBorderedSection(title, colorUnreachable, width, body)

	default: // StatusOK — may still be rendering stale data if Fetching after an error elsewhere
		ready, total := countReady(cs.Nodes)
		countText := fmt.Sprintf("%d/%d Ready", ready, total)
		borderColor := colorReady
		countStyled := styleReady.Render(countText)
		switch {
		case ready < total:
			borderColor = colorNotReady
			countStyled = styleNotReady.Render(countText + " ⚠")
		case hasWarning(cs.Nodes):
			borderColor = colorWarning
			countStyled = styleWarning.Render(countText)
		}
		title := styleHeader.Render(cs.Cluster.Name) + " ── " + countStyled
		innerWidth := width - clusterChrome
		body := renderNodeGrid(cs.Nodes, innerWidth)
		return renderBorderedSection(title, borderColor, width, body)
	}
}

// renderBorderedSection draws the box that makes a cluster's identity
// visible: a title line with the cluster name (and, for StatusOK, its
// ready/total count) embedded directly in the top border, then body wrapped
// in matching side borders, then a bottom border. Every cluster state
// (pending/unreachable/ok) goes through this same function so all 8 cluster
// sections look like siblings regardless of health.
func renderBorderedSection(title string, borderColor lipgloss.Color, width int, body string) string {
	if width < boxMinContent+clusterChrome {
		width = boxMinContent + clusterChrome
	}
	border := lipgloss.NewStyle().Foreground(borderColor)
	innerWidth := width - clusterChrome

	const prefix, suffix = "┌─ ", " "
	fillLen := width - lipgloss.Width(prefix) - lipgloss.Width(title) - lipgloss.Width(suffix) - 1 // -1 for the closing ┐
	if fillLen < 0 {
		fillLen = 0
	}
	top := border.Render(prefix) + title + border.Render(suffix+strings.Repeat("─", fillLen)+"┐")
	bottom := border.Render("└" + strings.Repeat("─", width-2) + "┘")

	var b strings.Builder
	b.WriteString(top)
	b.WriteString("\n")
	if body != "" {
		padded := lipgloss.NewStyle().Width(innerWidth).Render(body)
		for _, line := range strings.Split(padded, "\n") {
			b.WriteString(border.Render("│ "))
			b.WriteString(line)
			b.WriteString(border.Render(" │"))
			b.WriteString("\n")
		}
	}
	b.WriteString(bottom)
	return b.String()
}

func hasWarning(nodes []fetch.NodeRow) bool {
	for _, n := range nodes {
		if n.Warning != "" {
			return true
		}
	}
	return false
}

func countReady(nodes []fetch.NodeRow) (ready, total int) {
	for _, n := range nodes {
		if n.Ready {
			ready++
		}
	}
	return ready, len(nodes)
}
