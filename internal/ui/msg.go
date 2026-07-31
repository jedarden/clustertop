package ui

import (
	"time"

	"github.com/jedarden/clustertop/internal/fetch"
)

// tickMsg fires on the auto-refresh interval.
type tickMsg time.Time

// fetchResultMsg carries one cluster's fetch outcome, success or failure.
// Every fetchClusterCmd invocation returns exactly one of these — never
// nothing, never a panic.
type fetchResultMsg struct {
	ClusterName string
	Nodes       []fetch.NodeRow
	Err         error
}
