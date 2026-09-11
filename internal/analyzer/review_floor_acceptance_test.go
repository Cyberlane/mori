package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/source"
)

// Source-level scenario inspired by accessor-heavy APIs and visitor families.
// Lowering the floor should reveal their matches without letting the repeated
// scaffolding outrank the already-visible maintenance candidate.
func TestReviewFloorRetainsLogicAheadOfRepeatedAccessors(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		dir := filepath.Join(root, fmt.Sprintf("package%d", i))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		code := `export function convertWindow(value, scale, offset) {
    if (value < 0) { return offset; }
    let adjusted = value * scale + offset;
    for (let index = 0; index < 10; index++) {
      if (adjusted < offset) { adjusted += scale; } else { adjusted -= offset; }
    }
    if (scale === 0) { return offset + value; }
    if (adjusted > 100) { return 100; }
    return adjusted;
  }
  export function readValue(object, context, state, options) { return object.value; }
  export function callVisitor(object, node) { return object.visit(node); }
  export function setValue(object, value, context, state) { object.value = value; }
  export function rejectValue(value, context, state, options) { throw new Error(value); }
  export function smallArithmetic(value, context, state, options) { return value * value + 1; }
  export function smallCompound(object, value, context, state) { object.value += value; }
  export function smallGuard(value) { if (value) return value; return null; }
  `
		for _, name := range []string{"convertWindow", "smallArithmetic", "smallGuard", "smallCompound"} {
			code = strings.ReplaceAll(code, name, fmt.Sprintf("%s%d", name, i))
		}
		if err := os.WriteFile(filepath.Join(dir, "api.js"), []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
	}
	found := source.Discover([]string{root}, source.Options{MaxFileBytes: 1 << 20})
	options := Options{Threshold: 1, MinTokens: 40, Workers: 2, Ranking: RankingReview}
	high, err := Analyze(context.Background(), found.Files, found.Warnings, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(high.Groups) != 1 {
		t.Fatalf("high floor groups %d", len(high.Groups))
	}
	options.MinTokens = 12
	low, err := Analyze(context.Background(), found.Files, found.Warnings, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(low.Warnings) != 0 || len(low.Groups) < 5 {
		t.Fatalf("coverage: %d groups, warnings %v", len(low.Groups), low.Warnings)
	}
	if low.Groups[0].ID != high.Groups[0].ID {
		t.Fatalf("high-floor lead displaced by %s, signals %v", low.Groups[0].Profiles[0].Occurrences[0].Location.Name, low.Groups[0].ReviewSignals)
	}
	seen := map[string]bool{}
	for _, group := range low.Groups {
		name := group.Profiles[0].Occurrences[0].Location.Name
		penalized := strings.Contains(strings.Join(group.ReviewSignals, " "), "boilerplate")
		switch name {
		case "readValue", "callVisitor", "setValue", "rejectValue":
			seen[name] = true
			if !penalized {
				t.Errorf("%s lacked boilerplate evidence: %v", name, group.ReviewSignals)
			}
		default:
			if penalized {
				t.Errorf("logic demoted: %s", name)
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("missing boilerplate scenario coverage: %v", seen)
	}
	options.MaxGroups = 1
	shortlist, err := Analyze(context.Background(), found.Files, nil, options)
	if err != nil || !shortlist.Truncated || len(shortlist.Groups) != 1 || shortlist.Groups[0].ID != high.Groups[0].ID || shortlist.TotalMatchGroups != low.TotalMatchGroups {
		t.Fatalf("bounded shortlist lost lead/counts: %v %#v", err, shortlist)
	}
	options.MaxGroups = 0
	options.Ranking = RankingStructural
	structural, err := Analyze(context.Background(), found.Files, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	identities := func(r model.Report) map[string]float64 {
		m := map[string]float64{}
		for _, g := range r.Groups {
			m[g.ID] = g.Similarity
		}
		return m
	}
	if !reflect.DeepEqual(identities(low), identities(structural)) || low.TotalLocationPairs != structural.TotalLocationPairs {
		t.Fatal("ranking changed detection")
	}
}
