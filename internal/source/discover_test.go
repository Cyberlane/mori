package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverSupportedFilesAndDefaultExcludes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "main.go"), "package fixture\n")
	writeFixture(t, filepath.Join(root, "src", "check.py"), "def check():\n    return True\n")
	writeFixture(t, filepath.Join(root, "vendor", "ignored.go"), "package ignored\n")
	writeFixture(t, filepath.Join(root, "README.md"), "# ignored\n")

	result := Discover([]string{root}, Options{MaxFileBytes: 1024})
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", result.Warnings)
	}
	if len(result.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(result.Files))
	}
	if !strings.HasSuffix(result.Files[0].DisplayPath, "main.go") {
		t.Fatalf("first file = %q, want main.go", result.Files[0].DisplayPath)
	}
	if !strings.HasSuffix(result.Files[1].DisplayPath, "src/check.py") {
		t.Fatalf("second file = %q, want src/check.py", result.Files[1].DisplayPath)
	}
}

func TestDiscoverCustomExcludeAndExplicitWarning(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	included := filepath.Join(root, "main.go")
	excluded := filepath.Join(root, "main_test.go")
	unsupported := filepath.Join(root, "notes.md")
	writeFixture(t, included, "package fixture\n")
	writeFixture(t, excluded, "package fixture\n")
	writeFixture(t, unsupported, "# notes\n")

	result := Discover(
		[]string{root, unsupported},
		Options{Excludes: []string{"**/*_test.go"}},
	)
	if len(result.Files) != 1 || result.Files[0].Path != included {
		t.Fatalf("files = %#v, want only %q", result.Files, included)
	}
	if len(result.Warnings) != 1 ||
		result.Warnings[0].Message != "unsupported source extension" {
		t.Fatalf("warnings = %#v, want unsupported extension", result.Warnings)
	}
}

func TestDiscoverExcludesExplicitFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	excluded := filepath.Join(root, "main_test.go")
	writeFixture(t, excluded, "package fixture\n")

	result := Discover(
		[]string{excluded},
		Options{Excludes: []string{"**/*_test.go"}},
	)
	if len(result.Files) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v, want an excluded file with no warning", result)
	}
}

func TestDiscoverContextStopsBeforeWalking(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DiscoverContext(ctx, []string{"."}, Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DiscoverContext error = %v, want context.Canceled", err)
	}
}

func TestSymlinkComponentDetectionStopsAtBoundary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	writeFixture(t, filepath.Join(target, "source.go"), "package fixture\n")
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}
	hasSymlink, err := hasSymlinkComponent(filepath.Join(link, "source.go"), root)
	if err != nil {
		t.Fatalf("hasSymlinkComponent: %v", err)
	}
	if !hasSymlink {
		t.Fatal("hasSymlinkComponent = false, want true")
	}
}

func TestDiscoverRejectsSymlinkedAncestorOutsideWorkingDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "target")
	writeFixture(t, filepath.Join(target, "source.go"), "package fixture\n")
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}

	result := Discover([]string{filepath.Join(link, "source.go")}, Options{})
	if len(result.Files) != 0 || len(result.Warnings) != 1 {
		t.Fatalf("result = %#v, want no files and one warning", result)
	}
	if !strings.Contains(result.Warnings[0].Message, "symbolic links") {
		t.Fatalf("warning = %#v, want symbolic-link explanation", result.Warnings[0])
	}
}

func TestDiscoverWarningDoesNotRepeatAbsolutePath(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.go")
	result := Discover([]string{missing}, Options{})
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one", result.Warnings)
	}
	if strings.Contains(result.Warnings[0].Message, missing) {
		t.Fatalf("warning message %q contains absolute path", result.Warnings[0].Message)
	}
}

func TestValidatePatterns(t *testing.T) {
	t.Parallel()

	if err := ValidatePatterns([]string{"**/*_test.go"}); err != nil {
		t.Fatalf("valid pattern: %v", err)
	}
	if err := ValidatePatterns([]string{"["}); err == nil {
		t.Fatal("invalid pattern returned nil")
	}
}

func writeFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
