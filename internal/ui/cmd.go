package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jedarden/clustertop/internal/config"
	"github.com/jedarden/clustertop/internal/fetch"
)

// fetchClusterCmd always returns exactly one fetchResultMsg, success or
// failure — fetch.FetchClusterNodes never panics and never blocks past
// timeout, so this closure can't hang the Bubble Tea event loop.
func fetchClusterCmd(c config.Cluster, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		rows, err := fetch.FetchClusterNodes(context.Background(), c, timeout)
		return fetchResultMsg{ClusterName: c.Name, Nodes: rows, Err: err}
	}
}

// fetchAllCmd batches one Cmd per cluster. tea.Batch runs each on its own
// goroutine, so one slow/dead cluster only delays its own fetchResultMsg —
// the others land on schedule.
func fetchAllCmd(clusters []config.Cluster, timeout time.Duration) tea.Cmd {
	cmds := make([]tea.Cmd, len(clusters))
	for i, c := range clusters {
		cmds[i] = fetchClusterCmd(c, timeout)
	}
	return tea.Batch(cmds...)
}

func tickCmd(every time.Duration) tea.Cmd {
	return tea.Tick(every, func(t time.Time) tea.Msg { return tickMsg(t) })
}
