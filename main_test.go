package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	harness := newDashboardSmokeHarness(t)
	harness.writeConfig(t, workingDir)
	wantNodes := harness.nodeNames(t)

	cmd := exec.Command(binary)
	cmd.Dir = workingDir
	// Pin the CLI's terminal type so the assertions below see plain text
	// regardless of the invoking environment. exec.Cmd drops earlier
	// duplicates when it dedupes cmd.Env, so appending pins the child even
	// when the invoking shell exported a real TERM. TERM=dumb also stops the
	// startup background probe entirely: termenv's termStatusReport refuses
	// dumb/screen/tmux before writing anything, so bubbletea's package-init
	// lipgloss.HasDarkBackground call never sends its OSC 11 + DSR query.
	// Under any other TERM the query is written and blocks ~5s for a reply a
	// bare PTY master never sends — past cliStartupTimeout.
	cmd.Env = append(os.Environ(), "TERM=dumb")
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
	answeredBackgroundProbe := false
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				outputMu.Lock()
				output.Write(buf[:n])
				// bubbletea's package init probes the terminal's background
				// color (OSC 11 plus a DSR sentinel) and blocks for up to
				// termenv's 5s OSC timeout waiting for the reply. A real
				// terminal answers from the far end of the pty; answer here
				// too, or the CLI's first frame renders only after the probe
				// times out — past cliStartupTimeout — and the assertions
				// below fail no matter how healthy the fetch path is. This is
				// a safety net only: the TERM=dumb pin above stops the probe
				// from being sent at all.
				if !answeredBackgroundProbe && strings.Contains(output.String(), "\x1b]11;?") {
					answeredBackgroundProbe = true
					_, _ = ptmx.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\\x1b[1;1R"))
				}
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
	for clusterName, nodeName := range wantNodes {
		waitForOutput("┌─ " + clusterName)
		waitForOutput(nodeName)
	}
	harness.waitForRequests(t, 1)
	harness.assertRequests(t, 1)

	if _, err := ptmx.Write([]byte("r")); err != nil {
		t.Fatalf("send refresh key: %v", err)
	}
	for _, nodeName := range wantNodes {
		waitForOutput(nodeName)
	}
	harness.waitForRequests(t, 2)
	harness.assertRequests(t, 2)

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
