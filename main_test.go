package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	cmd := exec.Command("go", "build", "-o", binary, ".")
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
