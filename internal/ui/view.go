package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"

	"github.com/jedarden/clustertop/internal/fetch"
)

// wideBreakpoint is the terminal-width threshold (columns) above which the
// full column set renders. Below it, POOL/TYPE and VERSION drop and the
// NODE column truncates harder — see docs/plan/plan.md section 1's mockups.
const wideBreakpoint = 100

// columnsForWidth picks the column set for the current terminal width.
func columnsForWidth(width int) []table.Column {
	if width >= wideBreakpoint {
		return clusterTableColumns
	}
	return []table.Column{
		{Title: "NODE", Width: 16},
		{Title: "STATUS", Width: 12},
		{Title: "ROLES", Width: 10},
		{Title: "AGE", Width: 6},
	}
}

// narrowRow drops the POOL/TYPE and VERSION cells to match columnsForWidth's
// narrow column set.
func narrowRow(full table.Row) table.Row {
	// full is [NODE, STATUS, ROLES, POOL/TYPE, VERSION, AGE]
	return table.Row{full[0], full[1], full[2], full[5]}
}

// rowsForWidth builds table rows shaped to match columnsForWidth(width) —
// row cell count must always agree with the current column count.
func rowsForWidth(nodes []fetch.NodeRow, width int) []table.Row {
	rows := toTableRows(nodes)
	if width >= wideBreakpoint {
		return rows
	}
	narrow := make([]table.Row, len(rows))
	for i, r := range rows {
		narrow[i] = narrowRow(r)
	}
	return narrow
}

func (m Model) View() string {
	if m.Quitting {
		return ""
	}

	footer := styleFooter.Render("[q] quit  [r] refresh")

	if !m.ready {
		// Before the first WindowSizeMsg arrives, render un-viewported —
		// there's no known size to wrap a viewport to yet.
		return renderBody(m.Clusters) + "\n" + footer + "\n"
	}

	return m.Viewport.View() + "\n" + footer + "\n"
}

// renderBody renders every cluster section, without the top-level header or
// footer — this is what gets set as the viewport's scrollable content.
func renderBody(clusters []ClusterState) string {
	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("clustertop — %d clusters", len(clusters))))
	b.WriteString("\n\n")
	for _, cs := range clusters {
		b.WriteString(renderClusterSection(cs))
		b.WriteString("\n")
	}
	return b.String()
}

func renderClusterSection(cs ClusterState) string {
	var b strings.Builder

	switch cs.Status {
	case StatusPending:
		b.WriteString(styleHeader.Render(cs.Cluster.Name))
		b.WriteString(styleStale.Render(" — connecting…"))
		b.WriteString("\n")
		return b.String()

	case StatusError:
		b.WriteString(styleHeader.Render(cs.Cluster.Name))
		b.WriteString(" — ")
		b.WriteString(styleUnreachable.Render("UNREACHABLE"))
		b.WriteString("\n")
		staleness := ""
		if !cs.LastFetch.IsZero() {
			staleness = fmt.Sprintf(" (last seen %s ago)", time.Since(cs.LastFetch).Round(time.Second))
		}
		errText := ""
		if cs.Err != nil {
			errText = cs.Err.Error()
		}
		b.WriteString(styleNotReady.Render(fmt.Sprintf("  ⚠ %s%s", errText, staleness)))
		b.WriteString("\n")
		return b.String()

	default: // StatusOK — may still be rendering stale data if Fetching after an error elsewhere
		ready, total := countReady(cs.Nodes)
		header := fmt.Sprintf("%s — %d/%d Ready", cs.Cluster.Name, ready, total)
		if ready < total {
			header += " ⚠"
		}
		b.WriteString(styleHeader.Render(header))
		b.WriteString("\n")
		b.WriteString(cs.Table.View())
		b.WriteString("\n")
		return b.String()
	}
}

func countReady(nodes []fetch.NodeRow) (ready, total int) {
	for _, n := range nodes {
		if n.Ready {
			ready++
		}
	}
	return ready, len(nodes)
}
