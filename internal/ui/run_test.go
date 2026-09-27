package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
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
