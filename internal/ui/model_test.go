package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jedarden/clustertop/internal/config"
	"github.com/jedarden/clustertop/internal/fetch"
)

func newTestModel(clusterNames ...string) Model {
	cfg := config.Config{}
	for _, n := range clusterNames {
		cfg.Clusters = append(cfg.Clusters, config.Cluster{Name: n, Endpoint: "http://example.invalid:8001"})
	}
	return NewModel(cfg, 0, 0)
}

func TestUpdate_SuccessThenErrorPreservesStaleNodes(t *testing.T) {
	m := newTestModel("a")

	okMsg := fetchResultMsg{ClusterName: "a", Nodes: []fetch.NodeRow{{Name: "n1", Ready: true}}}
	newM, _ := m.Update(okMsg)
	m = newM.(Model)

	if m.Clusters[0].Status != StatusOK {
		t.Fatalf("expected StatusOK after success, got %v", m.Clusters[0].Status)
	}
	if len(m.Clusters[0].Nodes) != 1 {
		t.Fatalf("expected 1 node after success, got %d", len(m.Clusters[0].Nodes))
	}

	errMsg := fetchResultMsg{ClusterName: "a", Err: errors.New("boom")}
	newM, _ = m.Update(errMsg)
	m = newM.(Model)

	if m.Clusters[0].Status != StatusError {
		t.Fatalf("expected StatusError after failure, got %v", m.Clusters[0].Status)
	}
	if len(m.Clusters[0].Nodes) != 1 {
		t.Fatalf("expected the prior successful Nodes to be preserved (stale-but-visible), got %d nodes", len(m.Clusters[0].Nodes))
	}
	if m.Clusters[0].Nodes[0].Name != "n1" {
		t.Fatalf("expected preserved node to still be n1, got %q", m.Clusters[0].Nodes[0].Name)
	}
}

func TestUpdate_UnknownClusterNameIgnored(t *testing.T) {
	m := newTestModel("a")
	msg := fetchResultMsg{ClusterName: "does-not-exist", Nodes: []fetch.NodeRow{{Name: "x"}}}
	newM, cmd := m.Update(msg)
	got := newM.(Model)
	if cmd != nil {
		t.Errorf("expected nil cmd for an unknown cluster name, got %v", cmd)
	}
	if got.Clusters[0].Status != StatusPending {
		t.Errorf("expected the known cluster to be untouched (still Pending), got %v", got.Clusters[0].Status)
	}
}

func TestHandleKey_QuitReturnsTeaQuit(t *testing.T) {
	m := newTestModel("a")
	newM, cmd, ok := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !ok {
		t.Fatal("expected 'q' to match a clustertop binding")
	}
	if !newM.Quitting {
		t.Error("expected Quitting to be true after 'q'")
	}
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd for quit")
	}
}

func TestHandleKey_RefreshMarksAllFetching(t *testing.T) {
	m := newTestModel("a", "b")
	newM, cmd, ok := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if !ok {
		t.Fatal("expected 'r' to match a clustertop binding")
	}
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd (fetchAllCmd) for refresh")
	}
	for i, cs := range newM.Clusters {
		if !cs.Fetching {
			t.Errorf("cluster %d: expected Fetching=true after refresh", i)
		}
	}
}

func TestHandleKey_UnmatchedKeyReturnsNotOK(t *testing.T) {
	m := newTestModel("a")
	_, _, ok := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if ok {
		t.Error("expected an unmatched key (up arrow) to return ok=false so it falls through to the viewport")
	}
}

func TestWindowSizeMsg_ResizesViewportAndReflowsGrid(t *testing.T) {
	m := newTestModel("a")
	m.Clusters[0].Nodes = []fetch.NodeRow{{Name: "n1", Ready: true}}

	wide, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	wm := wide.(Model)
	if wm.Viewport.Width != 200 {
		t.Errorf("expected viewport width 200, got %d", wm.Viewport.Width)
	}
	wideCols, wideContent := gridLayout(200 - clusterChrome)
	narrowCols, narrowContent := gridLayout(60 - clusterChrome)
	if wideCols <= narrowCols && wideContent <= narrowContent {
		t.Errorf("expected a 200-wide section to lay out looser than a 60-wide one (cols %d/%d, content %d/%d)",
			wideCols, narrowCols, wideContent, narrowContent)
	}

	narrow, _ := wm.Update(tea.WindowSizeMsg{Width: 60, Height: 50})
	nm := narrow.(Model)
	if nm.Viewport.Width != 60 {
		t.Errorf("expected viewport width 60, got %d", nm.Viewport.Width)
	}
}
