package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jedarden/clustertop/internal/config"
)

const (
	defaultClustersPath = "clusters.yaml"
	defaultRefreshEvery = 15 * time.Second
	defaultFetchTimeout = 5 * time.Second
)

// Run starts the TUI, loading clusters.yaml from the current directory.
func Run() error {
	cfg, err := config.LoadClusters(defaultClustersPath)
	if err != nil {
		return err
	}
	model := NewModel(cfg, defaultRefreshEvery, defaultFetchTimeout)
	_, err = tea.NewProgram(model).Run()
	return err
}
