package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jedarden/clustertop/internal/config"
	"github.com/jedarden/clustertop/internal/fetch"
)

// ClusterStatus is tri-state, not bool: "never fetched yet" and "confirmed
// unreachable" must render differently, or a cold start is indistinguishable
// from a real incident. See docs/research/bubbletea-fault-isolation.md.
type ClusterStatus int

const (
	StatusPending ClusterStatus = iota
	StatusOK
	StatusError
)

// ClusterState is one cluster's last-known display state. Nodes is only ever
// overwritten on a successful fetch — an error flips Status but leaves the
// last known-good Nodes untouched, so a flaky cluster degrades to "stale"
// rather than blanking.
type ClusterState struct {
	Cluster   config.Cluster
	Status    ClusterStatus
	Nodes     []fetch.NodeRow
	Err       error
	LastFetch time.Time
	Fetching  bool
	Table     table.Model
}

const footerHeight = 1

// Model is the top-level Bubble Tea model for the whole dashboard.
type Model struct {
	Clusters     []ClusterState
	idx          map[string]int
	Width        int
	Height       int
	Viewport     viewport.Model
	ready        bool // true once the first WindowSizeMsg has set real dimensions
	RefreshEvery time.Duration
	FetchTimeout time.Duration
	Quitting     bool
}

// NewModel builds the initial Model from a loaded cluster config.
func NewModel(cfg config.Config, refreshEvery, fetchTimeout time.Duration) Model {
	m := Model{
		Clusters:     make([]ClusterState, len(cfg.Clusters)),
		idx:          make(map[string]int, len(cfg.Clusters)),
		RefreshEvery: refreshEvery,
		FetchTimeout: fetchTimeout,
	}
	for i, c := range cfg.Clusters {
		m.Clusters[i] = ClusterState{
			Cluster: c,
			Status:  StatusPending,
			Table:   newClusterTable(),
		}
		m.idx[c.Name] = i
	}
	return m
}

func (m Model) Init() tea.Cmd {
	clusters := make([]config.Cluster, len(m.Clusters))
	for i, cs := range m.Clusters {
		clusters[i] = cs.Cluster
	}
	return tea.Batch(
		fetchAllCmd(clusters, m.FetchTimeout),
		tickCmd(m.RefreshEvery),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		m.Viewport.Width = msg.Width
		m.Viewport.Height = msg.Height - footerHeight
		m.ready = true
		m.applyLayout()
		return m, nil

	case tea.KeyMsg:
		if newM, cmd, ok := m.handleKey(msg); ok {
			return newM, cmd
		}
		// Not one of clustertop's own bindings — forward to the viewport so
		// its default scroll keys (up/down/pgup/pgdown) work.
		var vpCmd tea.Cmd
		m.Viewport, vpCmd = m.Viewport.Update(msg)
		return m, vpCmd

	case tickMsg:
		m.markAllFetching()
		clusters := make([]config.Cluster, len(m.Clusters))
		for i, cs := range m.Clusters {
			clusters[i] = cs.Cluster
		}
		return m, tea.Batch(fetchAllCmd(clusters, m.FetchTimeout), tickCmd(m.RefreshEvery))

	case fetchResultMsg:
		i, ok := m.idx[msg.ClusterName]
		if !ok {
			return m, nil
		}
		cs := &m.Clusters[i]
		cs.Fetching = false
		cs.LastFetch = time.Now()
		if msg.Err != nil {
			cs.Status = StatusError
			cs.Err = msg.Err
			// Nodes/Table intentionally left untouched — stale-but-visible.
		} else {
			cs.Status = StatusOK
			cs.Err = nil
			cs.Nodes = msg.Nodes
		}
		m.applyLayout()
		return m, nil
	}
	return m, nil
}

func (m *Model) markAllFetching() {
	for i := range m.Clusters {
		m.Clusters[i].Fetching = true
	}
}

// applyLayout re-derives every cluster table's columns and rows from the
// current terminal width and each cluster's last-known Nodes, then
// refreshes the viewport's content. Called after any resize or fetch
// result, so columns/rows and the rendered width never drift apart.
func (m *Model) applyLayout() {
	cols := columnsForWidth(m.Width)
	for i := range m.Clusters {
		cs := &m.Clusters[i]
		// bubbles/table re-renders on SetColumns using whatever rows are
		// currently stored — if those rows still have the OLD column count
		// (e.g. 6 cells from a wide layout) while cols has just been
		// narrowed to 4, renderRow panics indexing m.cols[i] out of range.
		// Clearing rows first, before the column count changes underneath
		// them, keeps row/column shape consistent at every intermediate
		// step. Caught live via tmux resize testing against the real fleet.
		cs.Table.SetRows(nil)
		cs.Table.SetColumns(cols)
		cs.Table.SetRows(rowsForWidth(cs.Nodes, m.Width))
	}
	if m.ready {
		m.Viewport.SetContent(renderBody(m.Clusters))
	}
}
