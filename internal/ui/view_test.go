package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jedarden/clustertop/internal/config"
	"github.com/jedarden/clustertop/internal/fetch"
)

// TestRenderClusterSection_AllLinesSameWidth guards the bug caught by manual
// inspection during development: lipgloss's Width() measures the padded
// content area, not the text area inside the padding, so a naively-computed
// box width silently wrapped node names onto an extra line and threw every
// box's right border out of alignment with its neighbors.
func TestRenderClusterSection_AllLinesSameWidth(t *testing.T) {
	cs := ClusterState{
		Cluster: config.Cluster{Name: "apexalgo-iad"},
		Status:  StatusOK,
		Nodes: []fetch.NodeRow{
			{Name: "memory1-30-a", Roles: "<none>", PoolType: "memory1-30", Version: "v1.33.0", Age: "45d", Ready: true},
			{Name: "prod-instance-17825485895130660", Roles: "<none>", PoolType: "compute1-4", Version: "v1.33.0", Age: "24h", Ready: false, Warning: "scale-down tainted"},
		},
	}

	for _, width := range []int{40, 60, 100, 160} {
		out := renderClusterSection(cs, width)
		for i, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got != width {
				t.Errorf("width %d: line %d rendered at width %d, want %d: %q", width, i, got, width, line)
			}
		}
	}
}

func TestRenderClusterSection_PendingHasNoBodyLines(t *testing.T) {
	cs := ClusterState{Cluster: config.Cluster{Name: "iad-ci"}, Status: StatusPending}
	out := renderClusterSection(cs, 60)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected top+bottom border only (2 lines) while pending, got %d:\n%s", len(lines), out)
	}
}

func TestRenderClusterSection_UnreachableCollapsesToSingleLine(t *testing.T) {
	cs := ClusterState{
		Cluster: config.Cluster{Name: "iad-kalshi"},
		Status:  StatusError,
		Err:     errors.New("dial tcp: context deadline exceeded"),
	}
	out := renderClusterSection(cs, 60)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected border+one error line+border (3 lines), got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[1], "dial tcp") {
		t.Errorf("expected the error message on the body line, got %q", lines[1])
	}
}

func TestRenderClusterSection_SingleNodeClusterStillGetsFullBorder(t *testing.T) {
	cs := ClusterState{
		Cluster: config.Cluster{Name: "ardenone-manager"},
		Status:  StatusOK,
		Nodes:   []fetch.NodeRow{{Name: "only-node", Ready: true}},
	}
	out := renderClusterSection(cs, 60)
	if !strings.HasPrefix(out, "┌") {
		t.Errorf("expected a top border even for a single-node cluster, got %q", out)
	}
	if !strings.Contains(out, "╭") {
		t.Errorf("expected the single node to still render inside its own box, got:\n%s", out)
	}
}
