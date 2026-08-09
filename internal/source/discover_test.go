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

func TestDiscoverExtensionlessScriptsByShebang(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	zsh := filepath.Join(root, "syncpick")
	bash := filepath.Join(root, "build")
	unsupported := filepath.Join(root, "fish-tool")
	writeFixture(t, zsh, "#!/bin/zsh\nrun() { print ok; }\n")
	writeFixture(t, bash, "#!/usr/bin/env -S bash -eu\nrun() { printf '%s\\n' ok; }\n")
	writeFixture(t, unsupported, "#!/usr/bin/env fish\nfunction run; echo ok; end\n")

	result := Discover([]string{root}, Options{MaxFileBytes: 1024})
	if len(result.Warnings) != 0 || len(result.Files) != 2 {
		t.Fatalf("result = %#v, want two supported scripts and no warnings", result)
	}
	if result.Files[0].Language.ID != "bash" || result.Files[1].Language.ID != "zsh" {
		t.Fatalf("languages = %s/%s, want bash/zsh", result.Files[0].Language.ID, result.Files[1].Language.ID)
	}

	explicit := Discover([]string{unsupported}, Options{})
	if len(explicit.Files) != 0 || len(explicit.Warnings) != 1 ||
		explicit.Warnings[0].Message != "unsupported source extension or shebang" {
		t.Fatalf("explicit unsupported script result = %#v", explicit)
	}
}

func TestDiscoverDoesNotUseShebangToOverrideExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "script.py")
	writeFixture(t, path, "#!/bin/zsh\ndef run():\n    return True\n")
	result := Discover([]string{path}, Options{})
	if len(result.Files) != 1 || result.Files[0].Language.ID != "python" || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v, want extension-selected Python", result)
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

func TestDiscoverFiltersComparisonDomainsBeforeFileChecks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	code := filepath.Join(root, "main.go")
	sql := filepath.Join(root, "schema.sql")
	writeFixture(t, code, "package fixture\n")
	writeFixture(t, sql, strings.Repeat("-", 2048))

	result := Discover([]string{root, sql}, Options{
		MaxFileBytes: 1024,
		ComparisonDomains: map[string]struct{}{
			"code": {},
		},
	})
	if len(result.Files) != 1 || result.Files[0].Path != code || len(result.Warnings) != 0 {
		t.Fatalf("domain-filtered result = %#v", result)
	}
}

func TestDiscoverSelectsPostgreSQLDialect(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "migration.sql")
	writeFixture(t, path, "SELECT 1;\n")
	result := Discover([]string{path}, Options{SQLDialect: "postgresql"})
	if len(result.Files) != 1 || result.Files[0].Language.ID != "postgresql" ||
		len(result.Warnings) != 0 {
		t.Fatalf("PostgreSQL discovery = %#v", result)
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

func TestDiscoverHonorsNestedGitAndMoriIgnoreRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, filepath.Join(root, ".gitignore"), "generated/\n!generated/keep.go\n")
	writeFixture(t, filepath.Join(root, ".moriignore"), "private.go\n")
	writeFixture(t, filepath.Join(root, "generated", "ignored.go"), "package ignored\n")
	writeFixture(t, filepath.Join(root, "generated", "keep.go"), "package keep\n")
	writeFixture(t, filepath.Join(root, "private.go"), "package private\n")
	writeFixture(t, filepath.Join(root, "src", ".gitignore"), "*.go\n!keep.go\n")
	writeFixture(t, filepath.Join(root, "src", "ignored.go"), "package ignored\n")
	writeFixture(t, filepath.Join(root, "src", "keep.go"), "package keep\n")

	result, err := DiscoverContext(
		context.Background(),
		[]string{root},
		Options{IgnoreFiles: true},
	)
	if err != nil {
		t.Fatalf("DiscoverContext: %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("files = %#v, want two re-included files", result.Files)
	}
	for _, file := range result.Files {
		if !strings.HasSuffix(file.DisplayPath, "keep.go") {
			t.Fatalf("unexpected discovered file %q", file.DisplayPath)
		}
	}
	if len(result.IgnoreFiles) != 3 {
		t.Fatalf("ignore files = %#v, want root git/mori and nested git", result.IgnoreFiles)
	}
}

func TestDiscoverIgnoreRulesDoNotHideExplicitFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	ignored := filepath.Join(root, "ignored.go")
	writeFixture(t, filepath.Join(root, ".gitignore"), "ignored.go\n")
	writeFixture(t, ignored, "package ignored\n")

	result, err := DiscoverContext(
		context.Background(),
		[]string{ignored},
		Options{IgnoreFiles: true},
	)
	if err != nil {
		t.Fatalf("DiscoverContext: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != ignored {
		t.Fatalf("explicit ignored file result = %#v", result)
	}
}

func TestDiscoverRejectsInvalidIgnorePattern(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, filepath.Join(root, ".moriignore"), "[\n")
	writeFixture(t, filepath.Join(root, "main.go"), "package main\n")
	_, err := DiscoverContext(
		context.Background(),
		[]string{root},
		Options{IgnoreFiles: true},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid ignore pattern") {
		t.Fatalf("DiscoverContext error = %v, want invalid ignore pattern", err)
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
