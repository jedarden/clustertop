package ui

import "github.com/charmbracelet/lipgloss"

// Raw colors, exported as lipgloss.Color so both text styles below and
// border-coloring code in box.go/view.go can share one palette instead of
// duplicating ANSI codes.
const (
	colorReady       = lipgloss.Color("42")  // green
	colorNotReady    = lipgloss.Color("196") // red
	colorWarning     = lipgloss.Color("214") // yellow
	colorPending     = lipgloss.Color("244") // gray — never fetched yet
	colorUnreachable = lipgloss.Color("196") // red
)

var (
	styleReady       = lipgloss.NewStyle().Foreground(colorReady)
	styleNotReady    = lipgloss.NewStyle().Foreground(colorNotReady)
	styleWarning     = lipgloss.NewStyle().Foreground(colorWarning)
	styleUnreachable = lipgloss.NewStyle().Foreground(colorUnreachable).Bold(true)
	styleHeader      = lipgloss.NewStyle().Bold(true)
	styleFooter      = lipgloss.NewStyle().Faint(true)
	styleStale       = lipgloss.NewStyle().Faint(true)
)
