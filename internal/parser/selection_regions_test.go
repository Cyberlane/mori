package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

func TestCInlineTestRegionsKeepProductionBranches(t *testing.T) {
	content := `int production(void) { return 1; }
#ifdef REDIS_TEST
int test_helper(void) { return 2; }
#ifdef OTHER
int nested_test(void) { return 3; }
#endif
#else
int production_fallback(void) { return 4; }
#endif
#ifndef REDIS_TEST
int release_only(void) { return 5; }
#else
int unclassified_else(void) { return 6; }
#endif
#if defined(REDIS_TEST)
int defined_test(void) { return 7; }
#endif
#if defined(REDIS_TEST) || defined(PRODUCTION)
int shared(void) { return 8; }
#endif
#ifdef SOME_TEST
int unknown_macro(void) { return 9; }
#endif
int tests_in_name(void) { return 10; }
`
	path := filepath.Join(t.TempDir(), "input.c")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	spec, _ := language.Detect(path)
	for _, tc := range []struct {
		selection       string
		count, excluded int
	}{{"all", 10, 0}, {"production", 7, 3}, {"tests", 3, 0}} {
		t.Run(tc.selection, func(t *testing.T) {
			fragments, warnings, coverage := FileWithCoverage(context.Background(), source.File{Path: path, DisplayPath: "src/input.c", Language: spec}, Options{MinTokens: 1, FragmentSelection: tc.selection})
			for _, fragment := range fragments {
				isTest := fragment.Location.Name == "test_helper" || fragment.Location.Name == "nested_test" || fragment.Location.Name == "defined_test"
				if tc.selection == "production" && isTest || tc.selection == "tests" && !isTest {
					t.Fatalf("wrong branch retained: %s", fragment.Location.Name)
				}
			}
			if len(warnings) != 0 || len(fragments) != tc.count || coverage.ExcludedTestFragments != tc.excluded {
				t.Fatalf("fragments=%d warnings=%+v coverage=%+v", len(fragments), warnings, coverage)
			}
		})
	}
}

func TestSelectionPathOverridesRecoverLibraryAPI(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "django", "test", "client.py")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("def request(value):\n    return value + 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec, _ := language.Detect(path)
	for _, tc := range []struct {
		name, selection   string
		production, tests []string
		count             int
	}{
		{"conventional", "production", nil, nil, 0},
		{"shipped api", "production", []string{filepath.Dir(path)}, nil, 1},
		{"exact file", "production", []string{path}, nil, 1},
		{"boundary", "production", []string{filepath.Join(root, "django", "tes")}, nil, 0},
		{"tests excludes shipped api", "tests", []string{filepath.Dir(path)}, nil, 0},
		{"all unchanged", "all", nil, []string{filepath.Dir(path)}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fragments, warnings, _ := FileWithCoverage(context.Background(), source.File{Path: path, DisplayPath: "django/test/client.py", Language: spec}, Options{MinTokens: 1, FragmentSelection: tc.selection, ProductionPaths: tc.production, TestPaths: tc.tests})
			if len(warnings) != 0 || len(fragments) != tc.count {
				t.Fatalf("fragments=%d warnings=%+v", len(fragments), warnings)
			}
		})
	}
	// The reverse override makes a non-conventional integration support directory
	// explicitly test-only, without touching the adjacent production file.
	if !selectionPathMatches(filepath.Join(root, "integration", "helper.py"), filepath.Join(root, "integration")) || selectionPathMatches(filepath.Join(root, "integration-api", "helper.py"), filepath.Join(root, "integration")) {
		t.Fatal("path boundary matching failed")
	}
}

func TestValidateSelectionPathsRejectsAmbiguousRules(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		production, tests []string
		invalid           bool
	}{
		{[]string{filepath.Join(root, "django", "test")}, []string{filepath.Join(root, "tests")}, false},
		{[]string{root}, []string{filepath.Join(root, "tests")}, true},
		{[]string{filepath.Join(root, "tests")}, []string{root}, true},
		{[]string{root}, []string{root}, true},
		{[]string{"relative"}, nil, true},
	} {
		if err := ValidateSelectionPaths(tc.production, tc.tests); (err != nil) != tc.invalid {
			t.Fatalf("%+v error=%v", tc, err)
		}
	}
}

func TestSelectionPathAliasesAndIndexOnlyFiles(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "actual")
	if err := os.Mkdir(actual, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(actual, alias); err != nil {
		t.Skip(err)
	}
	if got, want := CanonicalSelectionPath(filepath.Join(alias, "new", "index.py")), CanonicalSelectionPath(filepath.Join(actual, "new", "index.py")); got != want {
		t.Fatalf("missing suffix: %q != %q", got, want)
	}
	if err := ValidateSelectionPaths([]string{actual}, []string{filepath.Join(alias, "tests")}); err == nil {
		t.Fatal("alias overlap accepted")
	}
}

func TestSelectionFilesystemRootMatchesChildren(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.py")
	root := filepath.VolumeName(path) + string(filepath.Separator)
	if !selectionPathMatches(path, root) {
		t.Fatal("root rule lost descendants")
	}
	if err := ValidateSelectionPaths([]string{root}, []string{filepath.Dir(path)}); err == nil {
		t.Fatal("root overlap accepted")
	}
}
