package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/table"
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

// Model is the top-level Bubble Tea model for the whole dashboard.
type Model struct {
	Clusters     []ClusterState
	idx          map[string]int
	Width        int
	Height       int
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
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

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
			cs.Table.SetRows(toTableRows(msg.Nodes))
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) markAllFetching() {
	for i := range m.Clusters {
		m.Clusters[i].Fetching = true
	}
}
