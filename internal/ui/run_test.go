package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jedarden/clustertop/internal/config"
	"github.com/jedarden/clustertop/internal/fetch"
)

func TestDefaultTimingBudget(t *testing.T) {
	if defaultRefreshEvery != 15*time.Second {
		t.Fatalf("defaultRefreshEvery = %v, want 15s", defaultRefreshEvery)
	}
	if defaultFetchTimeout != 10*time.Second {
		t.Fatalf("defaultFetchTimeout = %v, want 10s", defaultFetchTimeout)
	}
	if defaultFetchTimeout >= defaultRefreshEvery {
		t.Fatalf("defaultFetchTimeout = %v must remain below refresh interval %v", defaultFetchTimeout, defaultRefreshEvery)
	}
}

func TestDefaultFetchTimeoutAllowsHealthyNineSecondResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(9 * time.Second)
		_, _ = w.Write([]byte(`{"items": [{"metadata": {"name": "iad-ci-node"}}]}`))
	}))
	defer srv.Close()

	rows, err := fetch.FetchClusterNodes(context.Background(), config.Cluster{
		Name:     "iad-ci",
		Endpoint: srv.URL,
	}, defaultFetchTimeout)
	if err != nil {
		t.Fatalf("healthy response near observed iad-ci latency returned an error: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "iad-ci-node" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

// TestRunReturnsDuplicateClusterNameErrorBeforeStartingTUI pins the startup
// wiring: Run must surface config.LoadClusters's duplicate-name rejection
// verbatim and fail fast, never handing control to the Bubble Tea loop. If
// Run instead ignored the error and started the TUI, this test would hang
// waiting for terminal input until the timeout below fired.
func TestRunReturnsDuplicateClusterNameErrorBeforeStartingTUI(t *testing.T) {
	dir := t.TempDir()
	duplicateClustersYAML := []byte(`clusters:
  - name: iad-ci
    endpoint: http://127.0.0.1:1
    route: /api/v1/nodes
  - name: iad-ci
    endpoint: http://127.0.0.1:2
    route: /api/v1/nodes
`)
	if err := os.WriteFile(filepath.Join(dir, defaultClustersPath), duplicateClustersYAML, 0o644); err != nil {
		t.Fatalf("write clusters.yaml fixture: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir into fixture dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore Chdir: %v", err)
		}
	})

	done := make(chan error, 1)
	go func() {
		done <- Run()
	}()

	const wantErr = `config: duplicate cluster name "iad-ci"`
	select {
	case err := <-done:
		if err == nil || err.Error() != wantErr {
			t.Fatalf("Run() error = %v, want %q", err, wantErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return promptly; it may have entered the Bubble Tea loop instead of failing fast on the duplicate-name config error")
	}
}
