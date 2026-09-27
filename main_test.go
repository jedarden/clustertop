package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
)

const cliStartupTimeout = 5 * time.Second

func buildCLI(t *testing.T) string {
	t.Helper()

	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot, err := filepath.Abs(filepath.Dir(source))
	if err != nil {
		t.Fatalf("find repository root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "clustertop")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	return binary
}

func writeClustersYAML(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "clusters.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write clusters.yaml: %v", err)
	}
}

func runCLI(t *testing.T, binary, dir string) (string, *exec.ExitError) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), cliStartupTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("CLI exited successfully, want a configuration error; output:\n%s", output)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("CLI error = %T %v, want an exit error; output:\n%s", err, err, output)
	}
	if ctx.Err() != nil {
		t.Fatalf("CLI did not exit before the startup timeout: %v", ctx.Err())
	}
	return string(output), exitErr
}

func assertCLIConfigError(t *testing.T, binary, dir, want string) {
	t.Helper()

	output, exitErr := runCLI(t, binary, dir)
	if exitErr.ExitCode() != 1 {
		t.Fatalf("CLI exit code = %d, want 1; output:\n%s", exitErr.ExitCode(), output)
	}
	if !strings.Contains(output, want) {
		t.Fatalf("CLI output = %q, want it to contain %q", output, want)
	}
}

func TestCLIStartupMissingClustersYAMLExitsWithLoadError(t *testing.T) {
	binary := buildCLI(t)
	workingDir := t.TempDir()

	output, exitErr := runCLI(t, binary, workingDir)
	if exitErr.ExitCode() != 1 {
		t.Fatalf("CLI exit code = %d, want 1; output:\n%s", exitErr.ExitCode(), output)
	}
	for _, want := range []string{"clustertop:", "clusters.yaml", "no such file or directory"} {
		if !strings.Contains(output, want) {
			t.Errorf("CLI output = %q, want it to contain %q", output, want)
		}
	}
}

func TestCLIStartupEmptyClustersListIsRejected(t *testing.T) {
	binary := buildCLI(t)
	workingDir := t.TempDir()
	writeClustersYAML(t, workingDir, "clusters: []\n")

	assertCLIConfigError(t, binary, workingDir, "clustertop: config: clusters list is empty")
}

func TestCLIStartupReadsConfigurationFromCurrentWorkingDirectory(t *testing.T) {
	binary := buildCLI(t)
	workingDir := t.TempDir()
	writeClustersYAML(t, workingDir, "clusters: []\n")

	// Put a different, valid configuration beside the executable. The empty
	// list error proves that the command used the file in workingDir instead
	// of deriving its configuration path from the executable location.
	executableDir := filepath.Dir(binary)
	writeClustersYAML(t, executableDir, `clusters:
  - name: executable-directory-config
    endpoint: http://127.0.0.1:1
    route: test
`)

	assertCLIConfigError(t, binary, workingDir, "clustertop: config: clusters list is empty")
}

func TestCLIStartupHealthyDashboardFetchesRendersRefreshesAndQuits(t *testing.T) {
	binary := buildCLI(t)
	workingDir := t.TempDir()

	clusters := []struct {
		name          string
		initialNode   string
		refreshedNode string
		server        *httptest.Server
		requests      atomic.Int32
	}{
		{name: "alpha", initialNode: "alpha-node", refreshedNode: "alpha-new"},
		{name: "beta", initialNode: "beta-node", refreshedNode: "beta-new"},
	}
	for i := range clusters {
		cluster := &clusters[i]
		cluster.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/nodes" {
				cluster.requests.Add(1)
				http.Error(w, "unexpected request", http.StatusNotFound)
				return
			}

			requestNumber := cluster.requests.Add(1)
			nodeName := cluster.initialNode
			if requestNumber > 1 {
				nodeName = cluster.refreshedNode
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"apiVersion":"v1","kind":"NodeList","items":[{"metadata":{"name":"`+nodeName+`","labels":{"node-role.kubernetes.io/worker":"","node.kubernetes.io/instance-type":"compute1-4"}},"status":{"conditions":[{"type":"Ready","status":"True"}],"nodeInfo":{"kubeletVersion":"v1.32.0"}}}]}`)
		}))
		t.Cleanup(cluster.server.Close)
	}

	writeClustersYAML(t, workingDir, "clusters:\n"+
		"  - name: alpha\n"+
		"    endpoint: "+clusters[0].server.URL+"\n"+
		"    route: test\n"+
		"  - name: beta\n"+
		"    endpoint: "+clusters[1].server.URL+"\n"+
		"    route: test\n")

	cmd := exec.Command(binary)
	cmd.Dir = workingDir
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 160, Rows: 80})
	if err != nil {
		t.Fatalf("start CLI in PTY: %v", err)
	}
	defer func() {
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = ptmx.Close()
	}()

	var outputMu sync.Mutex
	var output strings.Builder
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				outputMu.Lock()
				output.Write(buf[:n])
				outputMu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}()

	waitForOutput := func(want string) {
		t.Helper()
		deadline := time.Now().Add(cliStartupTimeout)
		for time.Now().Before(deadline) {
			outputMu.Lock()
			got := output.String()
			outputMu.Unlock()
			if strings.Contains(got, want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		outputMu.Lock()
		got := output.String()
		outputMu.Unlock()
		t.Fatalf("CLI output did not contain %q; output:\n%s", want, got)
	}

	waitForOutput("[q] quit  [r] refresh")
	waitForOutput("┌─ alpha")
	waitForOutput("┌─ beta")
	waitForOutput("alpha-node")
	waitForOutput("beta-node")
	for i := range clusters {
		cluster := &clusters[i]
		if got := cluster.requests.Load(); got != 1 {
			t.Fatalf("%s startup request count = %d, want exactly 1", cluster.name, got)
		}
	}

	if _, err := ptmx.Write([]byte("r")); err != nil {
		t.Fatalf("send refresh key: %v", err)
	}
	waitForOutput("alpha-new")
	waitForOutput("beta-new")
	for i := range clusters {
		cluster := &clusters[i]
		if got := cluster.requests.Load(); got != 2 {
			t.Fatalf("%s request count after refresh = %d, want exactly 2", cluster.name, got)
		}
	}

	if _, err := ptmx.Write([]byte("q")); err != nil {
		t.Fatalf("send quit key: %v", err)
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case err := <-waitCh:
		if err != nil {
			t.Fatalf("CLI exited after q: %v", err)
		}
	case <-time.After(cliStartupTimeout):
		t.Fatal("CLI did not exit after q before the timeout")
	}

	_ = ptmx.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("PTY output reader did not stop after CLI exit")
	}
}
