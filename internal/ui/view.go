package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/jedarden/clustertop/internal/fetch"
)

func (m Model) View() string {
	if m.Quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("clustertop — %d clusters", len(m.Clusters))))
	b.WriteString("\n\n")

	for _, cs := range m.Clusters {
		b.WriteString(renderClusterSection(cs))
		b.WriteString("\n")
	}

	b.WriteString(styleFooter.Render("[q] quit  [r] refresh"))
	b.WriteString("\n")
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
