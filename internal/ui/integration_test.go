package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jedarden/clustertop/internal/config"
)

// TestFaultIsolation_OneSlowClusterDoesNotBlockOthers is the single
// highest-value test in this project (docs/research/bubbletea-fault-isolation.md):
// it proves one dead/slow cluster can never block or blank the rest of the
// UI, without needing a real cluster.
func TestFaultIsolation_OneSlowClusterDoesNotBlockOthers(t *testing.T) {
	const healthyCount = 7
	const timeout = 200 * time.Millisecond

	var clusters []config.Cluster

	for i := 0; i < healthyCount; i++ {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"items": [{"metadata": {"name": "node-a"}}]}`))
		}))
		t.Cleanup(srv.Close)
		clusters = append(clusters, config.Cluster{Name: srv.URL, Endpoint: srv.URL})
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // well past `timeout`
		w.Write([]byte(`{"items": []}`))
	}))
	t.Cleanup(slow.Close)
	clusters = append(clusters, config.Cluster{Name: "slow-cluster", Endpoint: slow.URL})

	start := time.Now()
	cmd := fetchAllCmd(clusters, timeout)
	msg := cmd() // tea.Batch's returned Cmd, run synchronously — dispatches all inner Cmds concurrently and waits for the batch

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected a tea.BatchMsg, got %T", msg)
	}

	results := make(map[string]fetchResultMsg)
	for _, c := range batch {
		m := c()
		fr, ok := m.(fetchResultMsg)
		if !ok {
			t.Fatalf("expected a fetchResultMsg from each batched Cmd, got %T", m)
		}
		results[fr.ClusterName] = fr
	}
	elapsed := time.Since(start)

	if elapsed > 2*timeout+time.Second {
		t.Fatalf("fault isolation failed: overall batch took %v, expected the healthy clusters to not wait on the slow one", elapsed)
	}

	healthy := 0
	for _, c := range clusters[:healthyCount] {
		r, ok := results[c.Name]
		if !ok {
			t.Fatalf("missing result for healthy cluster %s", c.Name)
		}
		if r.Err != nil {
			t.Errorf("healthy cluster %s: expected no error, got %v", c.Name, r.Err)
		}
		if len(r.Nodes) != 1 {
			t.Errorf("healthy cluster %s: expected 1 node, got %d", c.Name, len(r.Nodes))
		}
		healthy++
	}
	if healthy != healthyCount {
		t.Fatalf("expected %d healthy results, got %d", healthyCount, healthy)
	}

	slowResult, ok := results["slow-cluster"]
	if !ok {
		t.Fatal("missing result for the slow cluster")
	}
	if slowResult.Err == nil {
		t.Error("expected the slow cluster to report a timeout error, got nil")
	}
}
