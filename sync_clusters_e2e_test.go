package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const wantSyncClustersYAML = "clusters:\n" +
	"  - name: apexalgo-iad\n" +
	"    endpoint: http://traefik-apexalgo-iad.tail1b1987.ts.net:8001\n" +
	"    route: traefik-kubectl-tcp\n" +
	"  - name: iad-kalshi\n" +
	"    endpoint: http://kubectl-proxy-iad-kalshi.tail1b1987.ts.net:8001\n" +
	"    route: direct-tailscale-operator\n"

const e2eDirectProxyYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubectl-proxy
---
apiVersion: v1
kind: Service
metadata:
  name: kubectl-proxy
  annotations:
    tailscale.com/hostname: kubectl-proxy-iad-kalshi
spec:
  ports:
    - name: proxy
      port: 8001
`

const e2eTraefikProxyYAML = `apiVersion: v1
kind: Service
metadata:
  name: kubectl-proxy
spec:
  ports:
    - name: proxy
      port: 8001
`

const e2eTraefikServiceYAML = `apiVersion: v1
kind: Service
metadata:
  name: traefik-tailscale
  annotations:
    tailscale.com/hostname: traefik-apexalgo-iad
spec:
  ports:
    - name: vpn
      port: 8444
    - name: kubectl-tcp
      port: 8001
`

const e2eMalformedYAML = "{{{ not: [yaml: at all"

func runCLICommand(t *testing.T, binary, dir string, args ...string) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), cliStartupTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("CLI did not exit before the startup timeout: %v", ctx.Err())
	}
	return string(output), err
}

func writeSyncClustersFixtureFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func writeSyncClustersE2EFixture(t *testing.T, root string) {
	t.Helper()

	// Create the directories in reverse name order so output determinism is
	// tested independently of fixture creation order.
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "iad-kalshi", "devpod-observer", "kubectl-proxy.yml"), e2eDirectProxyYAML)
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "apexalgo-iad", "devpod-observer", "kubectl-proxy.yml"), e2eTraefikProxyYAML)
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "apexalgo-iad", "traefik", "tailscale-service.yml"), e2eTraefikServiceYAML)

	// These entries are all present in a declarative-config-shaped tree but
	// must not appear in the generated file.
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "broken", "devpod-observer", "kubectl-proxy.yml"), e2eMalformedYAML)
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "no-hostname", "devpod-observer", "kubectl-proxy.yml"), e2eTraefikProxyYAML)
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "no-hostname", "traefik", "tailscale-service.yml"), strings.Replace(e2eTraefikServiceYAML, "traefik-apexalgo-iad", "", 1))
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "portless", "devpod-observer", "kubectl-proxy.yml"), e2eTraefikProxyYAML)
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "portless", "traefik", "tailscale-service.yml"), strings.Replace(e2eTraefikServiceYAML, "    - name: kubectl-tcp\n      port: 8001\n", "", 1))
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "argocd", "root-app.yml"), "apiVersion: argoproj.io/v1alpha1\nkind: Application\n")
	writeSyncClustersFixtureFile(t, filepath.Join(root, "k8s", "retired", "ardenone-hub", ".gitkeep"), "")
}

func sourceCheckoutRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Dir(source))
	if err != nil {
		t.Fatalf("find source checkout: %v", err)
	}
	return root
}

func TestSyncClustersCLIRequiresDeclarativeConfigPath(t *testing.T) {
	binary := buildCLI(t)
	output, err := runCLICommand(t, binary, t.TempDir(), "sync-clusters")
	if err == nil {
		t.Fatalf("sync-clusters succeeded without its required path flag; output:\n%s", output)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("CLI error = %T %v; output:\n%s", err, err, output)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("CLI exit code = %d, want 1; output:\n%s", exitErr.ExitCode(), output)
	}
	if !strings.Contains(output, "sync-clusters: --declarative-config-path is required") {
		t.Fatalf("CLI output = %q, want the required-path error", output)
	}
}

func TestSyncClustersCLIDefaultOutAndSourceCheckoutUnchanged(t *testing.T) {
	binary := buildCLI(t)
	workingDir := t.TempDir()
	declarativeConfig := filepath.Join(workingDir, "declarative-config")
	writeSyncClustersE2EFixture(t, declarativeConfig)

	sourceClustersPath := filepath.Join(sourceCheckoutRoot(t), "clusters.yaml")
	sourceBefore, err := os.ReadFile(sourceClustersPath)
	if err != nil {
		t.Fatalf("read source checkout clusters.yaml before CLI: %v", err)
	}

	output, err := runCLICommand(t, binary, workingDir,
		"sync-clusters", "--declarative-config-path", declarativeConfig)
	if err != nil {
		t.Fatalf("sync-clusters failed: %v\noutput:\n%s", err, output)
	}

	got, err := os.ReadFile(filepath.Join(workingDir, "clusters.yaml"))
	if err != nil {
		t.Fatalf("read default output path: %v", err)
	}
	if string(got) != wantSyncClustersYAML {
		t.Errorf("default output =\n%swant =\n%s", got, wantSyncClustersYAML)
	}

	sourceAfter, err := os.ReadFile(sourceClustersPath)
	if err != nil {
		t.Fatalf("read source checkout clusters.yaml after CLI: %v", err)
	}
	if !bytes.Equal(sourceBefore, sourceAfter) {
		t.Errorf("sync-clusters modified the source checkout's clusters.yaml")
	}
}

func TestSyncClustersCLIWithCustomOutSkipsInvalidManifestsAndIsDeterministic(t *testing.T) {
	binary := buildCLI(t)
	workingDir := t.TempDir()
	declarativeConfig := filepath.Join(workingDir, "declarative-config")
	writeSyncClustersE2EFixture(t, declarativeConfig)
	outputDir := filepath.Join(workingDir, "generated")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("create custom output directory: %v", err)
	}
	firstOut := filepath.Join(outputDir, "first.yaml")
	secondOut := filepath.Join(outputDir, "second.yaml")

	for _, out := range []string{firstOut, secondOut} {
		output, err := runCLICommand(t, binary, workingDir,
			"sync-clusters",
			"--declarative-config-path", declarativeConfig,
			"--out", out,
		)
		if err != nil {
			t.Fatalf("sync-clusters with custom --out %s failed: %v\noutput:\n%s", out, err, output)
		}
		for _, skipped := range []string{"broken", "no-hostname", "portless"} {
			if !strings.Contains(output, "sync-clusters: skipping "+skipped) {
				t.Errorf("CLI output = %q, want a skip warning for %s", output, skipped)
			}
		}
	}

	first, err := os.ReadFile(firstOut)
	if err != nil {
		t.Fatalf("read first custom output: %v", err)
	}
	second, err := os.ReadFile(secondOut)
	if err != nil {
		t.Fatalf("read second custom output: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("repeated CLI runs produced different output:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if string(first) != wantSyncClustersYAML {
		t.Errorf("custom output =\n%swant =\n%s", first, wantSyncClustersYAML)
	}
	if _, err := os.Stat(filepath.Join(workingDir, "clusters.yaml")); !os.IsNotExist(err) {
		t.Errorf("custom --out run created the default output path, stat error = %v", err)
	}
}
