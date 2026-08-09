package language

import (
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestEveryGrammarHasCompatibleABI(t *testing.T) {
	t.Parallel()

	expectedABI := map[string]uint32{
		"bash":       15,
		"go":         15,
		"javascript": 15,
		"python":     15,
		"postgresql": 15,
		"rust":       15,
		"sql":        14,
		"swift":      14,
		"tsx":        14,
		"typescript": 14,
		"zsh":        15,
	}

	for _, spec := range All() {
		spec := spec
		t.Run(spec.ID, func(t *testing.T) {
			t.Parallel()

			language := spec.NewLanguage()
			if got, want := language.AbiVersion(), expectedABI[spec.ID]; got != want {
				t.Fatalf("%s ABI = %d, want %d", spec.ID, got, want)
			}
			if got := language.AbiVersion(); got < tree_sitter.MIN_COMPATIBLE_LANGUAGE_VERSION ||
				got > tree_sitter.LANGUAGE_VERSION {
				t.Fatalf(
					"%s ABI = %d, supported range is %d-%d",
					spec.ID,
					got,
					tree_sitter.MIN_COMPATIBLE_LANGUAGE_VERSION,
					tree_sitter.LANGUAGE_VERSION,
				)
			}
			parser := tree_sitter.NewParser()
			defer parser.Close()
			if err := parser.SetLanguage(language); err != nil {
				t.Fatalf("SetLanguage(%s): %v", spec.ID, err)
			}
		})
	}
}

func TestDetectWithSQLDialect(t *testing.T) {
	t.Parallel()

	for dialect, expected := range map[string]string{
		SQLDialectGeneric:    "sql",
		SQLDialectPostgreSQL: "postgresql",
	} {
		spec, ok := DetectWithSQLDialect("migration.SQL", dialect)
		if !ok || spec.ID != expected {
			t.Errorf("DetectWithSQLDialect(%q) = %q/%t, want %q/true", dialect, spec.ID, ok, expected)
		}
	}
	spec, ok := DetectWithSQLDialect("main.go", SQLDialectPostgreSQL)
	if !ok || spec.ID != "go" {
		t.Fatalf("non-SQL dialect detection = %q/%t, want go/true", spec.ID, ok)
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"build.bash":    "bash",
		"install.SH":    "bash",
		"main.go":       "go",
		"view.JSX":      "javascript",
		"worker.mts":    "typescript",
		"component.tsx": "tsx",
		"module.pyi":    "python",
		"lib.rs":        "rust",
		"queries.SQL":   "sql",
		"service.SWIFT": "swift",
		"plugin.zsh":    "zsh",
	}
	for path, expected := range tests {
		spec, ok := Detect(path)
		if !ok {
			t.Fatalf("Detect(%q) returned unsupported", path)
		}
		if spec.ID != expected {
			t.Errorf("Detect(%q) ID = %q, want %q", path, spec.ID, expected)
		}
	}

	if _, ok := Detect("README.md"); ok {
		t.Fatal("Detect(README.md) unexpectedly returned a language")
	}
}

func TestDetectShebang(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"#!/bin/sh":                  "bash",
		"#!/usr/bin/env bash":        "bash",
		"#!/usr/bin/env -S bash -eu": "bash",
		"#!/usr/bin/env -- zsh":      "zsh",
		"#!/opt/homebrew/bin/zsh -f": "zsh",
		"#!/usr/bin/env node":        "javascript",
		"#!/usr/bin/python3 -u":      "python",
	}
	for line, expected := range tests {
		spec, ok := DetectShebang(line)
		if !ok || spec.ID != expected {
			t.Errorf("DetectShebang(%q) = %q/%t, want %q/true", line, spec.ID, ok, expected)
		}
	}

	for _, line := range []string{"", "bin/zsh", "#!/bin/fish", "#!/usr/bin/env", "#!/bin/zsh\x00"} {
		if _, ok := DetectShebang(line); ok {
			t.Errorf("DetectShebang(%q) unexpectedly returned a language", line)
		}
	}
}

func TestAllReturnsIndependentSpecifications(t *testing.T) {
	t.Parallel()

	first := All()
	first[0].Extensions[0] = ".mutated"
	first[0].Shebangs[0] = "mutated"
	first[0].fragmentKinds["mutated"] = struct{}{}
	first[0].topLevelKinds["mutated"] = struct{}{}

	second := All()
	if second[0].Extensions[0] == ".mutated" || second[0].Shebangs[0] == "mutated" ||
		second[0].IsFragmentBoundary("mutated") {
		t.Fatal("mutating All result changed the registry")
	}
	if _, exists := second[0].topLevelKinds["mutated"]; exists {
		t.Fatal("mutating top-level kinds changed the registry")
	}
}

func TestComparisonDomainsAndFragmentKinds(t *testing.T) {
	t.Parallel()

	for _, spec := range All() {
		if spec.Family == "sql" {
			if spec.ComparisonDomain != "sql-query" || spec.FragmentKind != "query" {
				t.Fatalf("SQL spec = %#v", spec)
			}
			continue
		}
		if spec.ComparisonDomain != "code" || spec.FragmentKind != "function" {
			t.Fatalf("code spec %s = %#v", spec.ID, spec)
		}
		kinds := spec.FragmentKinds()
		if spec.Family == "shell" {
			if len(kinds) != 3 || kinds[0] != "block" || kinds[1] != "function" || kinds[2] != "script" {
				t.Fatalf("shell fragment kinds for %s = %#v", spec.ID, kinds)
			}
		} else if len(kinds) != 2 || kinds[0] != "block" || kinds[1] != "function" {
			t.Fatalf("code fragment kinds for %s = %#v", spec.ID, kinds)
		}
	}
}

func TestComparisonDomainsAreUniqueAndSorted(t *testing.T) {
	t.Parallel()

	domains := ComparisonDomains()
	if len(domains) != 2 || domains[0] != "code" || domains[1] != "sql-query" {
		t.Fatalf("ComparisonDomains() = %#v", domains)
	}
}

func TestTypeScriptSelectorsIncludeTSXFamily(t *testing.T) {
	t.Parallel()

	resolved, ok := ResolveSelector("typescript")
	if !ok || len(resolved) != 2 || resolved[0] != "tsx" || resolved[1] != "typescript" {
		t.Fatalf("ResolveSelector(typescript) = %#v/%t", resolved, ok)
	}
	if _, ok := ResolveSelector("unknown"); ok {
		t.Fatal("unknown selector resolved successfully")
	}
}
