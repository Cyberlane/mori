package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/source"
)

// Exercise discovery, parsing, classification, comparison and coverage together.
// A shipped testing API is production code when the user explicitly says so.
func TestLibrarySelectionOverridePreservesNeighborCoverage(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"django/test/client.py", "runtime/client.py", "tests/client.py", "integration/client.py"} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("def request(value):\n    if value < 0:\n        return 0\n    return value + 1\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	found := source.Discover([]string{root}, source.Options{MaxFileBytes: 1 << 20})
	options := Options{Threshold: 1, MinTokens: 1, Workers: 2, FragmentSelection: "production", ProductionPaths: []string{filepath.Join(root, "django", "test")}, TestPaths: []string{filepath.Join(root, "integration")}}
	result, err := Analyze(context.Background(), found.Files, found.Warnings, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 || len(result.Groups) != 1 {
		t.Fatalf("warnings=%v groups=%d", result.Warnings, len(result.Groups))
	}
	names := map[string]bool{}
	for _, profile := range result.Groups[0].Profiles {
		for _, occurrence := range profile.Occurrences {
			names[occurrence.Location.Path] = true
		}
	}
	if len(names) != 2 {
		t.Fatalf("selected occurrences=%v", names)
	}
	for path := range names {
		if filepath.Base(filepath.Dir(path)) != "runtime" && filepath.Base(filepath.Dir(path)) != "test" {
			t.Fatalf("unexpected selected path %s", path)
		}
	}
	options.FragmentSelection = "all"
	all, err := Analyze(context.Background(), found.Files, found.Warnings, options)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, profile := range all.Groups[0].Profiles {
		count += len(profile.Occurrences)
	}
	if count != 4 {
		t.Fatalf("all selection lost coverage: %d", count)
	}
}
