package normalize_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/parser"
	"github.com/Cyberlane/mori/internal/similarity"
	"github.com/Cyberlane/mori/internal/source"
)

func TestRenamesStaySimilarAndDifferentLogicDoesNot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fixtures.js")
	content := `
function first(address) {
  const clean = address.trim();
  return clean.includes("@") && clean.includes(".");
}

function renamed(candidate) {
  const normalized = candidate.trim();
  return normalized.includes("@") && normalized.includes(".");
}

function total(values) {
  let result = 0;
  for (const value of values) {
    result += value;
  }
  return result;
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("JavaScript grammar not detected")
	}
	fragments, warnings := parser.File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "fixtures.js",
		Language:    spec,
	}, 1)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(fragments) != 3 {
		t.Fatalf("fragments = %d, want 3", len(fragments))
	}

	byName := make(map[string]model.Fragment, len(fragments))
	for _, fragment := range fragments {
		byName[fragment.Location.Name] = fragment
	}
	renamedScore, _, _ := similarity.WeightedJaccard(
		byName["first"].Features,
		byName["renamed"].Features,
	)
	if renamedScore < 0.95 {
		t.Fatalf("renamed score = %.3f, want at least 0.95", renamedScore)
	}

	unrelatedScore, _, _ := similarity.WeightedJaccard(
		byName["first"].Features,
		byName["total"].Features,
	)
	if unrelatedScore >= 0.60 {
		t.Fatalf("unrelated score = %.3f, want below 0.60", unrelatedScore)
	}
}

func TestSemanticOperationFamiliesAndNearbyNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		positive string
		nearby   string
		feature  string
	}{
		{name: "membership", positive: "value.includes('x')", nearby: "value.includesOther('x')", feature: "membership"},
		{name: "pattern", positive: "value.matches('x')", nearby: "value.matchesOther('x')", feature: "pattern-match"},
		{name: "length", positive: "len(value)", nearby: "lenOther(value)", feature: "length"},
		{name: "lowercase", positive: "value.toLowerCase()", nearby: "value.toLowerCaseOther()", feature: "lowercase"},
		{name: "uppercase", positive: "value.toUpperCase()", nearby: "value.toUpperCaseOther()", feature: "uppercase"},
		{name: "trim", positive: "value.trim()", nearby: "value.trimOther()", feature: "trim"},
		{name: "filter", positive: "value.filter(predicate)", nearby: "value.filterOther(predicate)", feature: "filter"},
		{name: "map", positive: "value.map(transform)", nearby: "value.mapOther(transform)", feature: "map"},
		{name: "reduce", positive: "value.reduce(reducer)", nearby: "value.reduceOther(reducer)", feature: "reduce"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), test.name+".js")
			content := "function positive(value) { return " + test.positive + "; }\n" +
				"function nearby(value) { return " + test.nearby + "; }\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, ok := language.Detect(path)
			if !ok {
				t.Fatal("JavaScript grammar not detected")
			}
			fragments, warnings := parser.File(context.Background(), source.File{
				Path:        path,
				DisplayPath: filepath.Base(path),
				Language:    spec,
			}, 1)
			if len(warnings) != 0 || len(fragments) != 2 {
				t.Fatalf(
					"warnings/fragments = %#v/%d, want none/2",
					warnings,
					len(fragments),
				)
			}

			byName := make(map[string]model.Fragment, len(fragments))
			for _, fragment := range fragments {
				byName[fragment.Location.Name] = fragment
			}
			feature := "semantic:" + test.feature
			if got := byName["positive"].Features[feature]; got != 2 {
				t.Errorf("positive %s count = %d, want 2", feature, got)
			}
			if got := byName["nearby"].Features[feature]; got != 0 {
				t.Errorf("nearby %s count = %d, want 0", feature, got)
			}
		})
	}
}
