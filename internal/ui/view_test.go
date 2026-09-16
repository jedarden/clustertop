package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestRenderClusterSection_UnreachableWithoutSnapshotHasOnlyErrorLine(t *testing.T) {
	setAscii(t)
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
	if strings.Contains(out, "stale") {
		t.Errorf("initial unreachable state must not be labeled stale:\n%s", out)
	}
}

func TestRenderClusterSection_UnreachableRendersStaleSnapshot(t *testing.T) {
	setAscii(t)
	cs := ClusterState{
		Cluster:   config.Cluster{Name: "iad-kalshi"},
		Status:    StatusError,
		Nodes:     []fetch.NodeRow{{Name: "last-known-a", Ready: true}, {Name: "last-known-b", Ready: false}},
		Err:       errors.New("dial tcp: context deadline exceeded"),
		LastFetch: time.Now().Add(-4 * time.Minute),
	}

	out := renderClusterSection(cs, 60)
	if !strings.Contains(out, "stale 4m ago") {
		t.Errorf("expected elapsed stale indicator, got:\n%s", out)
	}
	for _, want := range []string{"UNREACHABLE", "dial tcp", "last-known-a", "last-known-b", "╌"} {
		if !strings.Contains(out, want) {
			t.Errorf("stale unreachable render missing %q:\n%s", want, out)
		}
	}

	lines := strings.Split(out, "\n")
	if len(lines) != 11 {
		t.Fatalf("expected error line plus one 8-line stale node row, got %d lines:\n%s", len(lines), out)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 60 {
			t.Errorf("line %d rendered at width %d, want 60: %q", i, got, line)
		}
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

// healthyCluster / degradedCluster / warnedCluster build the three StatusOK
// health shapes the aggregate border has to tell apart.
func healthyCluster(name string) ClusterState {
	return ClusterState{
		Cluster: config.Cluster{Name: name},
		Status:  StatusOK,
		Nodes: []fetch.NodeRow{
			{Name: "node-a", Ready: true},
			{Name: "node-b", Ready: true},
		},
	}
}

func degradedCluster(name string) ClusterState {
	cs := healthyCluster(name)
	cs.Nodes[1].Ready = false
	return cs
}

func warnedCluster(name string) ClusterState {
	cs := healthyCluster(name)
	cs.Nodes[1].Warning = "scale-down tainted"
	return cs
}

// TestRenderClusterSection_AggregateBorderColors pins the border color of the
// cluster section to the cluster's aggregate health: green only when every
// node is ready and quiet, red when any node is down, yellow when nodes are
// up but carrying warnings, red for unreachable, gray while still connecting.
// The border is the first thing visible when scanning the dashboard, so a
// wrong color here misreports a whole cluster.
func TestRenderClusterSection_AggregateBorderColors(t *testing.T) {
	cases := []struct {
		name      string
		cs        ClusterState
		wantColor string
		notColor  string // a color that must NOT appear on the top border, when the point is an override
		wantText  string
	}{
		{
			name:      "all ready is green",
			cs:        healthyCluster("ardenone-cluster"),
			wantColor: ansiGreen,
			wantText:  "2/2 Ready",
		},
		{
			name:      "any node down is red with a warning marker",
			cs:        degradedCluster("ord-devimprint"),
			wantColor: ansiRed,
			notColor:  ansiGreen,
			wantText:  "1/2 Ready ⚠",
		},
		{
			name:      "all ready but warned is yellow without a warning marker",
			cs:        warnedCluster("iad-options"),
			wantColor: ansiYellow,
			notColor:  ansiGreen,
			wantText:  "2/2 Ready",
		},
		{
			name:      "unreachable is red",
			cs:        ClusterState{Cluster: config.Cluster{Name: "iad-kalshi"}, Status: StatusError, Err: errors.New("dial tcp: context deadline exceeded")},
			wantColor: ansiRed,
			wantText:  "UNREACHABLE",
		},
		{
			name:      "pending is gray",
			cs:        ClusterState{Cluster: config.Cluster{Name: "iad-ci"}, Status: StatusPending},
			wantColor: ansiGray,
			wantText:  "connecting…",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setANSI256(t)
			top := strings.Split(renderClusterSection(tc.cs, 60), "\n")[0]
			if !strings.Contains(top, tc.wantColor) {
				t.Errorf("top border should contain %q, got %q", tc.wantColor, top)
			}
			if !strings.Contains(top, tc.wantText) {
				t.Errorf("top border should contain %q, got %q", tc.wantText, top)
			}
			if tc.notColor != "" && strings.Contains(top, tc.notColor) {
				t.Errorf("top border must not contain %q — health state was overridden, got %q", tc.notColor, top)
			}
		})
	}
}

// A healthy cluster never renders a warning marker anywhere — not in the
// count and not in any node box.
func TestRenderClusterSection_HealthyClusterHasNoWarningMarkers(t *testing.T) {
	setAscii(t)
	out := renderClusterSection(healthyCluster("ardenone-cluster"), 60)
	if strings.Contains(out, "⚠") {
		t.Errorf("healthy cluster rendered a warning marker:\n%s", out)
	}
}

// The single node of a one-node cluster still sits fully enclosed: one box,
// wrapped by the cluster border above and below it.
func TestRenderClusterSection_SingleNodeFullyEnclosed(t *testing.T) {
	setAscii(t)
	cs := ClusterState{
		Cluster: config.Cluster{Name: "ardenone-manager"},
		Status:  StatusOK,
		Nodes:   []fetch.NodeRow{{Name: "only-node", Ready: true}},
	}
	out := renderClusterSection(cs, 60)
	lines := strings.Split(out, "\n")

	// top border + 8 box lines (6 content + 2 box border) + bottom border
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines (cluster border + one 8-line node box), got %d:\n%s", len(lines), out)
	}
	if got := strings.Count(out, "╭"); got != 1 {
		t.Errorf("expected exactly one node box, found %d box top corners:\n%s", got, out)
	}
	if !strings.HasPrefix(lines[0], "┌") || !strings.HasPrefix(lines[len(lines)-1], "└") {
		t.Errorf("node box should be enclosed by the cluster border, first line %q last line %q", lines[0], lines[len(lines)-1])
	}
}
