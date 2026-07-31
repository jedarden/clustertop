package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clusters.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadClusters_Valid(t *testing.T) {
	path := writeTemp(t, `
clusters:
  - name: apexalgo-iad
    endpoint: http://traefik-apexalgo-iad.tail1b1987.ts.net:8001
    route: traefik-kubectl-tcp
  - name: iad-kalshi
    endpoint: http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001
    route: direct-tailscale-operator
`)
	cfg, err := LoadClusters(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(cfg.Clusters))
	}
	if cfg.Clusters[0].Name != "apexalgo-iad" || cfg.Clusters[0].Endpoint != "http://traefik-apexalgo-iad.tail1b1987.ts.net:8001" {
		t.Errorf("unexpected first cluster: %+v", cfg.Clusters[0])
	}
}

func TestLoadClusters_UnknownKeyIgnored(t *testing.T) {
	path := writeTemp(t, `
clusters:
  - name: apexalgo-iad
    endpoint: http://traefik-apexalgo-iad.tail1b1987.ts.net:8001
    route: traefik-kubectl-tcp
    future_field: some-value-a-future-schema-added
`)
	cfg, err := LoadClusters(path)
	if err != nil {
		t.Fatalf("unexpected error decoding an unknown key: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(cfg.Clusters))
	}
}

func TestLoadClusters_MissingEndpointStillLoads(t *testing.T) {
	path := writeTemp(t, `
clusters:
  - name: apexalgo-iad
    route: traefik-kubectl-tcp
`)
	cfg, err := LoadClusters(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Clusters[0].Endpoint != "" {
		t.Errorf("expected empty endpoint, got %q", cfg.Clusters[0].Endpoint)
	}
}

func TestLoadClusters_EmptyListErrors(t *testing.T) {
	path := writeTemp(t, `clusters: []`)
	_, err := LoadClusters(path)
	if err == nil {
		t.Fatal("expected an error for an empty clusters list, got nil")
	}
}

func TestLoadClusters_FileNotFound(t *testing.T) {
	_, err := LoadClusters(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}
