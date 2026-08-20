package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanProjectCompatibilityGateCannotBeBypassedByNoConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".mori-version"), []byte(projectPinnedVersion()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc Sample() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--no-config", "--format", "json", root}, &stdout, &stderr)
	if code != exitUpgrade || stdout.Len() != 0 || !strings.Contains(stderr.String(), "project compatibility gate") ||
		!strings.Contains(stderr.String(), "project-contract") || !strings.Contains(stderr.String(), "agent-skill") {
		t.Fatalf("gate exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestProjectUpgradeApplySatisfiesScanGate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".mori-version"), []byte("v0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc Sample() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("upgrade exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code := Run(context.Background(), []string{"scan", "--no-config", "--format", "json", "--min-tokens", "1", root}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("gated scan exit = %d, stderr = %q", code, stderr.String())
	}
}

func TestManagedProjectRootsDoNotIncludeUnrelatedWorkingDirectory(t *testing.T) {
	managed := t.TempDir()
	if err := os.WriteFile(filepath.Join(managed, ".mori-version"), []byte(projectPinnedVersion()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(managed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	roots, err := managedProjectRoots(scanOptions{}, []string{external})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 0 {
		t.Fatalf("managed roots = %q, want none", roots)
	}
}
