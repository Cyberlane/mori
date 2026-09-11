package corpus_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Cyberlane/mori/internal/analyzer"
	"github.com/Cyberlane/mori/internal/corpus"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/source"
)

func TestActionabilityCorpusAndDiscoveryScopes(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture path")
	}
	root := filepath.Join(filepath.Dir(current), "..", "..", "corpus", "actionability")
	evaluation, err := corpus.Evaluate(context.Background(), root, filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(evaluation.Violations) > 0 {
		t.Fatalf("actionability violations: %v", evaluation.Violations)
	}
	if evaluation.CaseCount != 10 {
		t.Fatalf("cases=%d", evaluation.CaseCount)
	}
	scan := func(path string) model.Report {
		t.Helper()
		discovered := source.Discover([]string{path}, source.Options{MaxFileBytes: 1024 * 1024})
		report, err := analyzer.Analyze(context.Background(), discovered.Files, discovered.Warnings, analyzer.Options{Threshold: .85, MinTokens: 1, MaxGroups: 1000, MaxOccurrences: 100, MaxPairs: 10000, SameLanguageOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Warnings) > 0 {
			t.Fatalf("fixture warnings: %v", report.Warnings)
		}
		return report
	}
	whole := scan(root)
	scoped := scan(filepath.Join(root, "code"))
	hasName := func(report model.Report, name string) bool {
		for _, group := range report.Groups {
			for _, profile := range group.Profiles {
				for _, location := range profile.Occurrences {
					if location.Location.Name == name {
						return true
					}
				}
			}
		}
		return false
	}
	for _, name := range []string{"copy_linux", "copy_windows"} {
		if !hasName(whole, name) || !hasName(scoped, name) {
			t.Fatalf("useful production copy %s lost", name)
		}
	}
	if !hasName(whole, "assertAccountResponse") || hasName(scoped, "assertAccountResponse") {
		t.Fatal("scope did not deliberately select test helper findings")
	}
	if !hasName(scoped, "expected") {
		t.Fatal("path-only scope unexpectedly removed Rust inline tests")
	}
}
