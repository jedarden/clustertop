package ui

import "github.com/charmbracelet/lipgloss"

var (
	styleReady       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	styleNotReady    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	styleWarning     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // yellow
	styleUnreachable = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleHeader      = lipgloss.NewStyle().Bold(true)
	styleFooter      = lipgloss.NewStyle().Faint(true)
	styleStale       = lipgloss.NewStyle().Faint(true)
)
