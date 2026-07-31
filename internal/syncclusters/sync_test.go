package syncclusters

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// directOperatorProxyYAML mirrors the real iad-kalshi shape: the
// tailscale.com/hostname annotation lives directly on the kubectl-proxy
// Service.
const directOperatorProxyYAML = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
spec:
  replicas: 1
---
apiVersion: v1
kind: Service
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
  annotations:
    tailscale.com/expose: "true"
    tailscale.com/hostname: kubectl-proxy-example-cluster
spec:
  ports:
    - name: proxy
      port: 8001
`

// traefikRoutedProxyYAML mirrors the real apexalgo-iad shape: the
// kubectl-proxy Service itself carries no tailscale annotation.
const traefikRoutedProxyYAML = `
apiVersion: v1
kind: Service
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
spec:
  ports:
    - name: proxy
      port: 8001
`

const traefikTailscaleServiceYAML = `
apiVersion: v1
kind: Service
metadata:
  name: traefik-tailscale
  namespace: traefik
  annotations:
    tailscale.com/expose: "true"
    tailscale.com/hostname: "traefik-example-cluster"
spec:
  ports:
    - name: vpn
      port: 8444
    - name: kubectl-tcp
      port: 8001
`

func TestScan_DirectTailscaleOperatorShape(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "k8s", "kalshi-like", "devpod-observer", "kubectl-proxy.yml"), directOperatorProxyYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(cfg.Clusters), cfg.Clusters)
	}
	c := cfg.Clusters[0]
	if c.Name != "kalshi-like" {
		t.Errorf("Name = %q, want %q", c.Name, "kalshi-like")
	}
	if c.Endpoint != "http://kubectl-proxy-example-cluster.tail1b1987.ts.net:8001" {
		t.Errorf("Endpoint = %q", c.Endpoint)
	}
	if c.Route != "direct-tailscale-operator" {
		t.Errorf("Route = %q, want direct-tailscale-operator", c.Route)
	}
}

func TestScan_TraefikRoutedShape(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "k8s", "apexalgo-like", "devpod-observer", "kubectl-proxy.yml"), traefikRoutedProxyYAML)
	writeFile(t, filepath.Join(root, "k8s", "apexalgo-like", "traefik", "tailscale-service.yml"), traefikTailscaleServiceYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(cfg.Clusters), cfg.Clusters)
	}
	c := cfg.Clusters[0]
	if c.Endpoint != "http://traefik-example-cluster.tail1b1987.ts.net:8001" {
		t.Errorf("Endpoint = %q", c.Endpoint)
	}
	if c.Route != "traefik-kubectl-tcp" {
		t.Errorf("Route = %q, want traefik-kubectl-tcp", c.Route)
	}
}

func TestScan_UnrecognizedShapeSkippedNotFatal(t *testing.T) {
	root := t.TempDir()
	// Has a kubectl-proxy.yml, but the Service carries no tailscale
	// annotation AND there's no traefik/tailscale-service.yml sibling either
	// — neither known shape.
	writeFile(t, filepath.Join(root, "k8s", "weird-cluster", "devpod-observer", "kubectl-proxy.yml"), traefikRoutedProxyYAML)
	// A normal cluster alongside it, to confirm the scan continues past the skip.
	writeFile(t, filepath.Join(root, "k8s", "kalshi-like", "devpod-observer", "kubectl-proxy.yml"), directOperatorProxyYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected the unrecognized-shape cluster to be skipped (1 remaining), got %d: %+v", len(cfg.Clusters), cfg.Clusters)
	}
	if cfg.Clusters[0].Name != "kalshi-like" {
		t.Errorf("expected the recognized cluster to survive the scan, got %q", cfg.Clusters[0].Name)
	}
}

func TestScan_NonClusterDirIgnored(t *testing.T) {
	root := t.TempDir()
	// A loose directory with no kubectl-proxy manifest at all (like the
	// real repo's k8s/argocd/, which is a single Application file, not a
	// cluster).
	writeFile(t, filepath.Join(root, "k8s", "argocd", "some-application.yml"), "apiVersion: v1\nkind: Application\n")
	writeFile(t, filepath.Join(root, "k8s", "kalshi-like", "devpod-observer", "kubectl-proxy.yml"), directOperatorProxyYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected only the real cluster dir, got %d: %+v", len(cfg.Clusters), cfg.Clusters)
	}
}
