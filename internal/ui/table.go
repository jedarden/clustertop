package ui

import (
	"github.com/charmbracelet/bubbles/table"

	"github.com/jedarden/clustertop/internal/fetch"
)

var clusterTableColumns = []table.Column{
	{Title: "NODE", Width: 24},
	{Title: "STATUS", Width: 12},
	{Title: "ROLES", Width: 14},
	{Title: "POOL/TYPE", Width: 12},
	{Title: "VERSION", Width: 10},
	{Title: "AGE", Width: 6},
}

// newClusterTable builds one bubbles/table.Model for a cluster section.
// Deliberately never .Focus()ed — keybindings are minimal (q/r only), no
// per-row selection.
func newClusterTable() table.Model {
	return table.New(
		table.WithColumns(clusterTableColumns),
		table.WithRows(nil),
		table.WithFocused(false),
	)
}

func statusCell(row fetch.NodeRow) string {
	symbol := "●"
	label := "Ready"
	if !row.Ready {
		symbol = "⬤"
		label = "NotReady"
	}
	cell := symbol + " " + label
	if row.Warning != "" {
		cell += " ⚠ " + row.Warning
	}
	return cell
}

// toTableRows converts business-object rows (fetch.NodeRow) into
// bubbles/table widget rows. NOT the corev1.Node -> NodeRow mapping — that's
// internal/fetch's job. This is the later, presentation-only transformation.
func toTableRows(nodes []fetch.NodeRow) []table.Row {
	rows := make([]table.Row, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, table.Row{
			n.Name,
			statusCell(n),
			n.Roles,
			n.PoolType,
			n.Version,
			n.Age,
		})
	}
	return rows
}
