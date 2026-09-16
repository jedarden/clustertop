package syncclusters

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jedarden/clustertop/internal/config"
)

// The fixtures below mirror the two real manifest shapes in
// jedarden/declarative-config (see docs/research/sync-clusters-source-manifests.md)
// at full fidelity — leading comments, multi-document files, the auxiliary
// annotations and ports the real files carry — so a scanner change that only
// happens to work on the simplified shapes in sync_test.go still gets caught.

// representativeDirectProxyYAML mirrors k8s/iad-kalshi/devpod-observer/
// kubectl-proxy.yml: a Deployment followed by the Service, whose tailscale
// annotations (including proxy-class) make this a direct-Tailscale-operator
// cluster with no Traefik.
const representativeDirectProxyYAML = `# kubectl-proxy deployment. iad-kalshi-like cluster has no Traefik —
# exposed directly via the Tailscale operator.
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
  labels:
    app: kubectl-proxy
    component: cross-cluster-access
    target-cluster: iad-kalshi
spec:
  replicas: 1
  strategy:
    type: Recreate
---
# Exposed via the Tailscale operator; reachable at
# kubectl-proxy-iad-kalshi.tail1b1987.ts.net on the tailnet.
apiVersion: v1
kind: Service
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
  labels:
    app: kubectl-proxy
    component: cross-cluster-access
  annotations:
    tailscale.com/expose: "true"
    tailscale.com/hostname: kubectl-proxy-iad-kalshi
    tailscale.com/proxy-class: dockerhub-auth
spec:
  type: ClusterIP
  selector:
    app: kubectl-proxy
  ports:
    - name: proxy
      port: 8001
      targetPort: 8001
      protocol: TCP
`

// representativeTraefikProxyYAML mirrors k8s/apexalgo-iad/devpod-observer/
// kubectl-proxy.yml: the Service carries labels but no tailscale annotation
// — the hostname for a Traefik-routed cluster lives in the sibling
// traefik/tailscale-service.yml instead.
const representativeTraefikProxyYAML = `# kubectl-proxy Deployment — accessed via the Traefik kubectl-tcp entrypoint.
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
  labels:
    app: kubectl-proxy
    component: cross-cluster-access
    target-cluster: apexalgo-iad
---
apiVersion: v1
kind: Service
metadata:
  name: kubectl-proxy
  namespace: devpod-observer
  labels:
    app: kubectl-proxy
    component: cross-cluster-access
spec:
  type: ClusterIP
  selector:
    app: kubectl-proxy
  ports:
    - name: proxy
      port: 8001
      targetPort: 8001
`

// representativeTraefikTailscaleServiceYAML mirrors
// k8s/apexalgo-iad/traefik/tailscale-service.yml: one Tailscale ingress for
// the whole cluster, carrying several ports of which only kubectl-tcp is the
// read-only proxy route the scanner keys on.
const representativeTraefikTailscaleServiceYAML = `# Single Tailscale ingress for the entire cluster.
---
apiVersion: v1
kind: Service
metadata:
  name: traefik-tailscale
  namespace: traefik
  annotations:
    tailscale.com/expose: "true"
    tailscale.com/hostname: "traefik-apexalgo-iad"
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: traefik
  ports:
    - name: needle-otlp
      port: 4318
      targetPort: 4318
    - name: vpn
      port: 8444
      targetPort: 8444
    - name: kubectl-tcp
      port: 8001
      targetPort: 8001
    - name: kubectl-ss-tcp
      port: 8002
      targetPort: 8002
`

// writeFleetFixture builds a representative declarative-config checkout: two
// resolvable clusters (one per shape, deliberately NOT in alphabetical
// creation order), plus every directory shape that must be excluded — the
// loose argocd/ Application dir, a stray file directly under k8s/, and the
// k8s/retired/<name>/ tombstone a decommissioned cluster leaves behind
// (mirroring the real k8s/retired/botburrow-agents/.gitkeep).
func writeFleetFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Creation order is reversed relative to name order to prove the scan
	// does not depend on directory creation order.
	writeFile(t, filepath.Join(root, "k8s", "iad-kalshi", "devpod-observer", "kubectl-proxy.yml"), representativeDirectProxyYAML)
	writeFile(t, filepath.Join(root, "k8s", "apexalgo-iad", "devpod-observer", "kubectl-proxy.yml"), representativeTraefikProxyYAML)
	writeFile(t, filepath.Join(root, "k8s", "apexalgo-iad", "traefik", "tailscale-service.yml"), representativeTraefikTailscaleServiceYAML)
	// Excluded shapes:
	writeFile(t, filepath.Join(root, "k8s", "README.md"), "# not a cluster\n")
	writeFile(t, filepath.Join(root, "k8s", "argocd", "root-app.yml"), "apiVersion: argoproj.io/v1alpha1\nkind: Application\n")
	writeFile(t, filepath.Join(root, "k8s", "retired", "ardenone-hub", ".gitkeep"), "")
	return root
}

func fleetWant() config.Config {
	return config.Config{Clusters: []config.Cluster{
		{Name: "apexalgo-iad", Endpoint: "http://traefik-apexalgo-iad.tail1b1987.ts.net:8001", Route: "traefik-kubectl-tcp"},
		{Name: "iad-kalshi", Endpoint: "http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001", Route: "direct-tailscale-operator"},
	}}
}

func TestScan_RepresentativeFleetFixture(t *testing.T) {
	cfg, err := Scan(writeFleetFixture(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, fleetWant()) {
		t.Errorf("Scan() =\n%+v\nwant =\n%+v", cfg, fleetWant())
	}
}

func TestScan_OrderIsAlphabeticalAndRepeatable(t *testing.T) {
	root := writeFleetFixture(t)

	first, err := Scan(root)
	if err != nil {
		t.Fatalf("first scan: unexpected error: %v", err)
	}
	// clusters.yaml is read top-to-bottom by the UI, so the emitted order is
	// part of the output contract: sorted by directory name, independent of
	// creation order. os.ReadDir guarantees sorted entries — this pins that
	// guarantee to the output.
	names := make([]string, 0, len(first.Clusters))
	for _, c := range first.Clusters {
		names = append(names, c.Name)
	}
	want := []string{"apexalgo-iad", "iad-kalshi"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("cluster order = %v, want %v", names, want)
	}

	// A second scan of the same tree must produce the identical sequence —
	// regeneration is only safe to diff/commit if it is deterministic.
	second, err := Scan(root)
	if err != nil {
		t.Fatalf("second scan: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("repeated scan differs:\nfirst  = %+v\nsecond = %+v", first, second)
	}
}

// Regression for the ardenone-hub incident (see the package doc): a cluster
// whose manifests are gone must drop out of the next regeneration, not ride
// along in the old file forever.
func TestScan_DecommissionedClusterDropsOutOfNextScan(t *testing.T) {
	root := writeFleetFixture(t)

	before, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(before.Clusters) != 2 {
		t.Fatalf("expected 2 clusters before decommission, got %d: %+v", len(before.Clusters), before.Clusters)
	}

	// Decommission = the cluster's manifest directory is removed from
	// declarative-config.
	if err := os.RemoveAll(filepath.Join(root, "k8s", "iad-kalshi")); err != nil {
		t.Fatal(err)
	}

	after, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(after.Clusters) != 1 {
		t.Fatalf("expected 1 cluster after decommission, got %d: %+v", len(after.Clusters), after.Clusters)
	}
	if after.Clusters[0].Name != "apexalgo-iad" {
		t.Errorf("surviving cluster = %q, want apexalgo-iad", after.Clusters[0].Name)
	}
}

// The real repo parks decommissioned clusters under k8s/retired/<name>/. The
// retired/ directory itself has no devpod-observer/kubectl-proxy.yml, and the
// scan must not recurse into it and resurrect the retired cluster.
func TestScan_RetiredSubtreeNotRescanned(t *testing.T) {
	root := writeFleetFixture(t)
	// A tombstone that still contains full manifests — even then, retired
	// clusters must not reappear.
	writeFile(t, filepath.Join(root, "k8s", "retired", "botburrow-agents", "devpod-observer", "kubectl-proxy.yml"), representativeDirectProxyYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, fleetWant()) {
		t.Errorf("Scan() =\n%+v\nwant =\n%+v", cfg, fleetWant())
	}
}

const malformedYAML = "{{{ not: [yaml: at all"

func TestScan_MalformedProxyManifestSkippedNotFatal(t *testing.T) {
	root := writeFleetFixture(t)
	// A corrupted kubectl-proxy.yml must not abort the scan or emit a
	// half-decoded cluster — the directory is skipped like any other
	// unrecognized shape.
	writeFile(t, filepath.Join(root, "k8s", "broken-cluster", "devpod-observer", "kubectl-proxy.yml"), malformedYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("malformed manifest made Scan fatal: %v", err)
	}
	if !reflect.DeepEqual(cfg, fleetWant()) {
		t.Errorf("Scan() =\n%+v\nwant =\n%+v", cfg, fleetWant())
	}
}

// If the direct path is unreadable but the Traefik sibling is intact, the
// cluster still resolves through the second branch of the decision tree.
func TestScan_MalformedPrimaryManifestStillResolvesViaTraefik(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "k8s", "half-broken", "devpod-observer", "kubectl-proxy.yml"), malformedYAML)
	writeFile(t, filepath.Join(root, "k8s", "half-broken", "traefik", "tailscale-service.yml"), representativeTraefikTailscaleServiceYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(cfg.Clusters), cfg.Clusters)
	}
	if cfg.Clusters[0].Route != "traefik-kubectl-tcp" {
		t.Errorf("Route = %q, want traefik-kubectl-tcp", cfg.Clusters[0].Route)
	}
}

// findService stops at the first decode error, but a Service found before any
// malformed trailing document is returned immediately — trailing garbage in
// an otherwise-valid manifest must not poison resolution.
func TestScan_ServiceBeforeMalformedTrailingDocStillResolves(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "k8s", "messy-tail", "devpod-observer", "kubectl-proxy.yml"),
		strings.TrimPrefix(representativeDirectProxyYAML, "\n")+"---\n"+malformedYAML+"\n")

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(cfg.Clusters), cfg.Clusters)
	}
	if cfg.Clusters[0].Route != "direct-tailscale-operator" {
		t.Errorf("Route = %q, want direct-tailscale-operator", cfg.Clusters[0].Route)
	}
}

// A Traefik hostname without a kubectl-tcp port means the 8001 proxy route
// does not exist on that Tailscale node — emitting it would produce a
// plausible-looking but dead endpoint.
func TestScan_TraefikServiceWithoutKubectlTCPPortSkipped(t *testing.T) {
	noProxyPort := strings.Replace(representativeTraefikTailscaleServiceYAML,
		`    - name: kubectl-tcp
      port: 8001
      targetPort: 8001
`, "", 1)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "k8s", "portless", "devpod-observer", "kubectl-proxy.yml"), representativeTraefikProxyYAML)
	writeFile(t, filepath.Join(root, "k8s", "portless", "traefik", "tailscale-service.yml"), noProxyPort)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 0 {
		t.Errorf("expected the hostname-without-kubectl-tcp cluster to be skipped, got %+v", cfg.Clusters)
	}
}

// An empty hostname annotation must not produce an "http://.tail..." endpoint.
func TestScan_EmptyHostnameAnnotationSkipped(t *testing.T) {
	emptyHost := strings.Replace(representativeDirectProxyYAML,
		"tailscale.com/hostname: kubectl-proxy-iad-kalshi", `tailscale.com/hostname: ""`, 1)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "k8s", "nameless", "devpod-observer", "kubectl-proxy.yml"), emptyHost)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 0 {
		t.Errorf("expected the empty-hostname cluster to be skipped, got %+v", cfg.Clusters)
	}
}

// Both shapes resolvable — the kubectl-proxy Service is checked first, so the
// direct operator endpoint wins over the Traefik one.
func TestScan_DirectShapeTakesPrecedenceOverTraefik(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "k8s", "both-shapes", "devpod-observer", "kubectl-proxy.yml"), representativeDirectProxyYAML)
	writeFile(t, filepath.Join(root, "k8s", "both-shapes", "traefik", "tailscale-service.yml"), representativeTraefikTailscaleServiceYAML)

	cfg, err := Scan(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(cfg.Clusters), cfg.Clusters)
	}
	if cfg.Clusters[0].Route != "direct-tailscale-operator" {
		t.Errorf("Route = %q, want direct-tailscale-operator (kubectl-proxy checked first)", cfg.Clusters[0].Route)
	}
	if cfg.Clusters[0].Endpoint != "http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001" {
		t.Errorf("Endpoint = %q, want the direct operator endpoint", cfg.Clusters[0].Endpoint)
	}
}

func TestScan_MissingK8sDirIsAnError(t *testing.T) {
	root := t.TempDir()
	_, err := Scan(root)
	if err == nil {
		t.Fatal("expected an error scanning a checkout with no k8s/ directory")
	}
	if !strings.Contains(err.Error(), filepath.Join(root, "k8s")) {
		t.Errorf("error should name the path it failed to read, got: %v", err)
	}
}

// Regeneration must replace the previous file wholesale: a stale entry for a
// decommissioned cluster and a stale ordering do not survive a rewrite.
func TestRun_RegeneratesSortedConfigWithoutStaleClusters(t *testing.T) {
	root := writeFleetFixture(t)
	out := filepath.Join(root, "clusters.yaml")
	// Stale previous output: contains the decommissioned ardenone-hub entry
	// (its manifests are gone from the fixture) and lists iad-kalshi first.
	stale := "clusters:\n  - name: iad-kalshi\n    endpoint: http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001\n    route: direct-tailscale-operator\n  - name: ardenone-hub\n    endpoint: http://traefik-ardenone-hub.tail1b1987.ts.net:8001\n    route: traefik-kubectl-tcp\n"
	writeFile(t, out, stale)

	if err := Run([]string{"--declarative-config-path", root, "--out", out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading regenerated output: %v", err)
	}
	want := "clusters:\n" +
		"  - name: apexalgo-iad\n    endpoint: http://traefik-apexalgo-iad.tail1b1987.ts.net:8001\n    route: traefik-kubectl-tcp\n" +
		"  - name: iad-kalshi\n    endpoint: http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001\n    route: direct-tailscale-operator\n"
	if string(got) != want {
		t.Errorf("regenerated YAML =\n%swant =\n%s", got, want)
	}
}

func TestRun_RefusesToWriteWhenNoClustersResolve(t *testing.T) {
	root := t.TempDir()
	// Only excluded shapes present — nothing resolvable.
	writeFile(t, filepath.Join(root, "k8s", "retired", "ardenone-hub", ".gitkeep"), "")
	out := filepath.Join(root, "clusters.yaml")

	err := Run([]string{"--declarative-config-path", root, "--out", out})
	if err == nil {
		t.Fatal("expected a refusal to write an empty clusters.yaml")
	}
	if !strings.Contains(err.Error(), "found 0 clusters") {
		t.Errorf("error should explain the empty scan, got: %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Errorf("refused run must not leave an output file, stat err = %v", statErr)
	}
}

func TestRun_RequiresDeclarativeConfigPath(t *testing.T) {
	if err := Run(nil); err == nil || !strings.Contains(err.Error(), "--declarative-config-path is required") {
		t.Errorf("Run(nil) error = %v, want the required-flag error", err)
	}
}
