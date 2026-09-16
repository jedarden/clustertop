package config

import (
	"errors"
	"io/fs"
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
	if cfg.Clusters[0].Route != "traefik-kubectl-tcp" {
		t.Errorf("unexpected first cluster route: %q", cfg.Clusters[0].Route)
	}
	if cfg.Clusters[1].Name != "iad-kalshi" || cfg.Clusters[1].Route != "direct-tailscale-operator" {
		t.Errorf("unexpected second cluster: %+v", cfg.Clusters[1])
	}
}

// TestLoadClusters_ShippedClustersYAML loads the clusters.yaml that actually
// ships at the repo root — the same file syncclusters regenerates — and pins
// the per-entry contract the TUI and fetch layer depend on: every cluster has
// a name, an endpoint, and a route, and no two entries collide. A sync run
// that emitted a half-populated or duplicated entry would otherwise only show
// up as a blank or confusing row in the running TUI.
func TestLoadClusters_ShippedClustersYAML(t *testing.T) {
	// go test runs with the package directory as cwd; the repo root is two
	// levels up.
	cfg, err := LoadClusters(filepath.Join("..", "..", "clusters.yaml"))
	if err != nil {
		t.Fatalf("shipped clusters.yaml does not load: %v", err)
	}
	if len(cfg.Clusters) < 1 {
		t.Fatal("shipped clusters.yaml has no clusters")
	}
	seenNames := make(map[string]bool, len(cfg.Clusters))
	seenEndpoints := make(map[string]bool, len(cfg.Clusters))
	for _, c := range cfg.Clusters {
		if c.Name == "" {
			t.Errorf("cluster with empty name: %+v", c)
		}
		if c.Endpoint == "" {
			t.Errorf("cluster %q has an empty endpoint", c.Name)
		}
		if c.Route == "" {
			t.Errorf("cluster %q has an empty route", c.Name)
		}
		if seenNames[c.Name] {
			t.Errorf("duplicate cluster name %q", c.Name)
		}
		if seenEndpoints[c.Endpoint] {
			t.Errorf("duplicate endpoint %q (cluster %q)", c.Endpoint, c.Name)
		}
		seenNames[c.Name] = true
		seenEndpoints[c.Endpoint] = true
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

// TestLoadClusters_MissingEndpointStillLoads pins the decided loader
// contract (docs/plan/plan.md, cluster-config component: "error only if
// Clusters is empty"): a cluster entry with no endpoint is not a load error.
// Reachability is the fetch layer's per-row concern — one broken entry must
// degrade to that cluster's UNREACHABLE row, never blank out the fleet by
// failing the whole load.
func TestLoadClusters_MissingEndpointStillLoads(t *testing.T) {
	path := writeTemp(t, `
clusters:
  - name: apexalgo-iad
    endpoint: http://traefik-apexalgo-iad.tail1b1987.ts.net:8001
    route: traefik-kubectl-tcp
  - name: broken-cluster
    route: traefik-kubectl-tcp
`)
	cfg, err := LoadClusters(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(cfg.Clusters))
	}
	if cfg.Clusters[0].Endpoint == "" {
		t.Errorf("healthy cluster lost its endpoint: %+v", cfg.Clusters[0])
	}
	if cfg.Clusters[1].Endpoint != "" {
		t.Errorf("expected empty endpoint, got %q", cfg.Clusters[1].Endpoint)
	}
	if cfg.Clusters[1].Name != "broken-cluster" || cfg.Clusters[1].Route != "traefik-kubectl-tcp" {
		t.Errorf("entry with missing endpoint lost its other fields: %+v", cfg.Clusters[1])
	}
}

// TestLoadClusters_NonStringEndpointScalarCoerces pins what "invalid
// endpoint" means at this layer: yaml.v3 coerces a plain scalar (here an
// int) into the string field, so endpoint: 8001 loads as "8001" with no
// error. The loader does no URL validation by design — a scheme-less or
// garbage endpoint surfaces as that cluster's fetch error downstream, not as
// a config load failure. Only a structurally wrong YAML node type (a
// sequence or mapping where a scalar belongs) is a load error; see
// TestLoadClusters_MalformedYAML.
func TestLoadClusters_NonStringEndpointScalarCoerces(t *testing.T) {
	path := writeTemp(t, `
clusters:
  - name: iad-kalshi
    endpoint: 8001
    route: direct-tailscale-operator
`)
	cfg, err := LoadClusters(path)
	if err != nil {
		t.Fatalf("unexpected error for a scalar endpoint: %v", err)
	}
	if cfg.Clusters[0].Endpoint != "8001" {
		t.Errorf("expected the scalar coerced to string %q, got %q", "8001", cfg.Clusters[0].Endpoint)
	}
}

// TestLoadClusters_MalformedYAML pins the error contract for YAML the loader
// cannot decode, in both failure shapes:
//
//   - parse errors (a scanner or parser failure) surface yaml.v3's own
//     message verbatim;
//   - type errors (a well-formed document whose node types don't fit the
//     schema) aggregate under "yaml: unmarshal errors" with the offending
//     line.
//
// Determinism: loading the same bytes twice must produce the exact same
// message — the assertions compare both loads against the pinned string, so
// any nondeterminism (e.g. map-iteration order leaking into an aggregated
// type error) fails the test. Actionability: every pinned message names a
// line or a concrete value, so a typo in clusters.yaml can be located
// without a debugger. One honest caveat the exact pins document: yaml.v3's
// parser-stage errors report an approximate line (the "did not find
// expected '-' indicator" row fires after the shown line), while
// scanner-stage and type errors are exact.
func TestLoadClusters_MalformedYAML(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		// wantErrMsg is the exact, deterministic message LoadClusters must
		// return for these bytes.
		wantErrMsg string
	}{
		{
			name: "tab indentation is a scanner error naming the exact line",
			yaml: `
clusters:
	- name: iad-kalshi
	  endpoint: http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001
`,
			wantErrMsg: "yaml: line 3: found character that cannot start any token",
		},
		{
			name: "under-indented key is a parser error",
			yaml: `
clusters:
  - name: apexalgo-iad
    endpoint: http://traefik-apexalgo-iad.tail1b1987.ts.net:8001
   route: traefik-kubectl-tcp
`,
			wantErrMsg: "yaml: line 2: did not find expected '-' indicator",
		},
		{
			name: "endpoint as a sequence is a type error naming the line and target type",
			yaml: `
clusters:
  - name: iad-kalshi
    endpoint:
      - http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001
`,
			wantErrMsg: "yaml: unmarshal errors:\n  line 5: cannot unmarshal !!seq into string",
		},
		{
			name: "clusters as a scalar is a type error naming the line and target type",
			yaml: `
clusters: not-a-list
`,
			wantErrMsg: "yaml: unmarshal errors:\n  line 2: cannot unmarshal !!str `not-a-list` into []config.Cluster",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, tc.yaml)

			cfg, err := LoadClusters(path)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if err.Error() != tc.wantErrMsg {
				t.Errorf("error message\n  got:  %q\n  want: %q", err.Error(), tc.wantErrMsg)
			}
			if len(cfg.Clusters) != 0 {
				t.Errorf("expected an empty Config alongside the error, got %d clusters", len(cfg.Clusters))
			}

			// Determinism: a second load of the same bytes must yield the
			// identical message.
			_, err2 := LoadClusters(path)
			if err2 == nil {
				t.Fatal("second load unexpectedly succeeded")
			}
			if err2.Error() != err.Error() {
				t.Errorf("error is not deterministic:\n  first:  %q\n  second: %q", err.Error(), err2.Error())
			}
		})
	}
}

// TestLoadClusters_EmptyListErrors pins the loader's one hard error —
// "there is nothing to display" (docs/plan/plan.md, cluster-config
// component). Every shape of an empty document produces the identical
// message, which is the determinism pin; the message names the problem, not
// just a failure, which is the actionability pin.
func TestLoadClusters_EmptyListErrors(t *testing.T) {
	const wantErrMsg = "config: clusters list is empty"

	cases := []struct {
		name string
		yaml string
	}{
		{name: "explicit empty list", yaml: "clusters: []"},
		{name: "clusters key with no value", yaml: "clusters:"},
		{name: "empty document", yaml: ""},
		{name: "unknown key only", yaml: "something_else: true"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, tc.yaml)

			cfg, err := LoadClusters(path)
			if err == nil {
				t.Fatal("expected an error for an empty clusters list, got nil")
			}
			if err.Error() != wantErrMsg {
				t.Errorf("error message\n  got:  %q\n  want: %q", err.Error(), wantErrMsg)
			}
			if len(cfg.Clusters) != 0 {
				t.Errorf("expected an empty Config alongside the error, got %d clusters", len(cfg.Clusters))
			}
		})
	}
}

func TestLoadClusters_FileNotFound(t *testing.T) {
	_, err := LoadClusters(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
	// Actionability for callers: the error must wrap fs.ErrNotExist so a
	// caller (internal/ui.Run) can distinguish "no clusters.yaml here" from
	// "clusters.yaml is broken" and say so.
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected an error matching fs.ErrNotExist, got %v", err)
	}
}
