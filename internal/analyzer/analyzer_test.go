package analyzer

import (
	"context"
	"reflect"
	"testing"

	"github.com/Cyberlane/mori/internal/source"
)

func TestAnalyzeCrossLanguageExamples(t *testing.T) {
	t.Parallel()

	discovered := source.Discover(
		[]string{"../../examples/email-validation"},
		source.Options{MaxFileBytes: 1024 * 1024},
	)
	if len(discovered.Warnings) != 0 {
		t.Fatalf("discovery warnings = %#v", discovered.Warnings)
	}

	options := Options{
		Threshold:         0.70,
		MinTokens:         12,
		MaxMatches:        100,
		MaxPairs:          100,
		Workers:           2,
		CrossLanguageOnly: true,
	}
	first, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	second, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil {
		t.Fatalf("Analyze second run: %v", err)
	}

	if first.Files != 4 || first.Fragments != 4 {
		t.Fatalf("files/fragments = %d/%d, want 4/4", first.Files, first.Fragments)
	}
	if len(first.Matches) < 2 {
		t.Fatalf("matches = %d, want at least 2", len(first.Matches))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("analysis is not deterministic across identical runs")
	}

	top := first.Matches[0]
	if top.Left.Location.Language == top.Right.Location.Language {
		t.Fatalf("top match is not cross-language: %#v", top)
	}
	if top.Similarity < options.Threshold {
		t.Fatalf("top score = %f, below threshold", top.Similarity)
	}
	for _, match := range first.Matches {
		if match.ID == "" {
			t.Fatal("match has no stable ID")
		}
		if match.Left.Fingerprint == "" || match.Right.Fingerprint == "" {
			t.Fatalf("match has incomplete fragment identities: %#v", match)
		}
	}
}

func TestAnalyzeBoundsReportedMatches(t *testing.T) {
	t.Parallel()

	discovered := source.Discover(
		[]string{"../../examples/email-validation"},
		source.Options{MaxFileBytes: 1024 * 1024},
	)
	result, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold:         0.70,
		MinTokens:         12,
		MaxMatches:        1,
		MaxPairs:          100,
		Workers:           2,
		CrossLanguageOnly: true,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.TotalMatches < 2 {
		t.Fatalf("total matches = %d, want at least 2", result.TotalMatches)
	}
	if len(result.Matches) != 1 || !result.Truncated {
		t.Fatalf(
			"reported matches/truncated = %d/%t, want 1/true",
			len(result.Matches),
			result.Truncated,
		)
	}
}

func TestAnalyzeHonorsPairLimit(t *testing.T) {
	t.Parallel()

	discovered := source.Discover(
		[]string{"../../examples/email-validation"},
		source.Options{MaxFileBytes: 1024 * 1024},
	)
	_, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold:  0.10,
		MinTokens:  1,
		MaxMatches: 100,
		MaxPairs:   1,
		Workers:    1,
	})
	if err == nil {
		t.Fatal("Analyze returned nil after exceeding pair limit")
	}
}
