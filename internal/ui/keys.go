package ui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jedarden/clustertop/internal/config"
)

// keyMap is deliberately minimal — no per-row selection, since the tables
// are never focused.
type keyMap struct {
	Quit    key.Binding
	Refresh key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Refresh, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Refresh, k.Quit}}
}

var helpModel = help.New()

// handleKey applies clustertop's own keybindings. Returns ok=false when msg
// didn't match one of them, so the caller can forward it to the viewport.
func (m Model) handleKey(msg tea.KeyMsg) (out Model, cmd tea.Cmd, ok bool) {
	switch {
	case key.Matches(msg, keys.Quit):
		m.Quitting = true
		return m, tea.Quit, true
	case key.Matches(msg, keys.Refresh):
		m.markAllFetching()
		clusters := make([]config.Cluster, len(m.Clusters))
		for i, cs := range m.Clusters {
			clusters[i] = cs.Cluster
		}
		return m, fetchAllCmd(clusters, m.FetchTimeout), true
	}
	return m, nil, false
}
