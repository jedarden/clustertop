package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jedarden/clustertop/internal/config"
)

const dashboardSmokeFixtureDir = "testdata/dashboard-smoke"

type dashboardSmokeRequest struct {
	method string
	path   string
	query  string
}

type dashboardSmokeEndpoint struct {
	mu       sync.Mutex
	requests []dashboardSmokeRequest
}

func (e *dashboardSmokeEndpoint) record(r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, dashboardSmokeRequest{
		method: r.Method,
		path:   r.URL.Path,
		query:  r.URL.RawQuery,
	})
}

func (e *dashboardSmokeEndpoint) snapshot() []dashboardSmokeRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]dashboardSmokeRequest(nil), e.requests...)
}

type dashboardSmokeHarness struct {
	config    config.Config
	endpoints map[string]*dashboardSmokeEndpoint
}

func newDashboardSmokeHarness(t *testing.T) *dashboardSmokeHarness {
	t.Helper()

	fixturePath := filepath.Join(dashboardSmokeFixtureDir, "clusters.yaml")
	cfg, err := config.LoadClusters(fixturePath)
	if err != nil {
		t.Fatalf("load dashboard smoke fixture %s: %v", fixturePath, err)
	}
	if len(cfg.Clusters) < 2 {
		t.Fatalf("dashboard smoke fixture has %d clusters, want at least 2", len(cfg.Clusters))
	}

	h := &dashboardSmokeHarness{
		config:    cfg,
		endpoints: make(map[string]*dashboardSmokeEndpoint, len(cfg.Clusters)),
	}
	for i, cluster := range cfg.Clusters {
		cluster := cluster
		payloadPath := filepath.Join(dashboardSmokeFixtureDir, cluster.Name+"-nodes.json")
		payload, err := os.ReadFile(payloadPath)
		if err != nil {
			t.Fatalf("read %s payload: %v", cluster.Name, err)
		}
		endpoint := &dashboardSmokeEndpoint{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			endpoint.record(r)
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/nodes" || r.URL.RawQuery != "" {
				http.Error(w, "unexpected dashboard smoke request", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(payload)
		}))
		t.Cleanup(server.Close)

		h.config.Clusters[i].Endpoint = server.URL
		h.endpoints[cluster.Name] = endpoint
	}
	return h
}

func (h *dashboardSmokeHarness) writeConfig(t *testing.T, dir string) {
	t.Helper()

	data, err := yaml.Marshal(h.config)
	if err != nil {
		t.Fatalf("marshal dashboard smoke config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "clusters.yaml"), data, 0o644); err != nil {
		t.Fatalf("write dashboard smoke config: %v", err)
	}
}

func (h *dashboardSmokeHarness) assertRequests(t *testing.T, wantCount int) {
	t.Helper()

	for _, cluster := range h.config.Clusters {
		requests := h.endpoints[cluster.Name].snapshot()
		if len(requests) != wantCount {
			t.Errorf("%s request count = %d, want %d: %+v", cluster.Name, len(requests), wantCount, requests)
		}
		for _, request := range requests {
			if request.method != http.MethodGet || request.path != "/api/v1/nodes" || request.query != "" {
				t.Errorf("%s recorded unexpected request: %+v", cluster.Name, request)
			}
		}
	}
}

func (h *dashboardSmokeHarness) waitForRequests(t *testing.T, wantCount int) {
	t.Helper()

	deadline := time.Now().Add(cliStartupTimeout)
	for time.Now().Before(deadline) {
		allReady := true
		for _, cluster := range h.config.Clusters {
			if len(h.endpoints[cluster.Name].snapshot()) < wantCount {
				allReady = false
				break
			}
		}
		if allReady {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.assertRequests(t, wantCount)
}

func (h *dashboardSmokeHarness) nodeNames(t *testing.T) map[string]string {
	t.Helper()

	want := make(map[string]string, len(h.config.Clusters))
	for _, cluster := range h.config.Clusters {
		payloadPath := filepath.Join(dashboardSmokeFixtureDir, cluster.Name+"-nodes.json")
		payload, err := os.Open(payloadPath)
		if err != nil {
			t.Fatalf("open %s payload: %v", cluster.Name, err)
		}
		var list struct {
			Items []struct {
				Metadata struct {
					Name string `yaml:"name" json:"name"`
				} `yaml:"metadata" json:"metadata"`
			} `yaml:"items" json:"items"`
		}
		if err := yaml.NewDecoder(payload).Decode(&list); err != nil {
			_ = payload.Close()
			t.Fatalf("decode %s payload: %v", cluster.Name, err)
		}
		_ = payload.Close()
		if len(list.Items) != 1 || list.Items[0].Metadata.Name == "" {
			t.Fatalf("%s payload does not contain exactly one named node", cluster.Name)
		}
		want[cluster.Name] = list.Items[0].Metadata.Name
	}
	return want
}
