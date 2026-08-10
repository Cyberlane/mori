package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestTextEscapesTerminalControlCharacters(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := Text(&output, model.Report{
		SchemaVersion:      model.SchemaVersion,
		Threshold:          0.7,
		Files:              2,
		Fragments:          2,
		TotalMatchGroups:   1,
		TotalLocationPairs: 1,
		Groups: []model.MatchGroup{{
			ID:            "identity",
			Similarity:    1,
			LocationPairs: 1,
			Profiles: []model.FragmentProfile{{
				Fingerprint:     "same",
				OccurrenceCount: 2,
				Occurrences: []model.FragmentSummary{
					{Location: model.Location{
						Path:           "evil\npath.go",
						Language:       "go",
						LanguageFamily: "go",
						Name:           "name\x1b[31m",
						StartLine:      1,
						EndLine:        2,
					}},
					{Location: model.Location{
						Path:           "safe.go",
						Language:       "go",
						LanguageFamily: "go",
						Name:           "safe",
						StartLine:      1,
						EndLine:        2,
					}},
				},
			}},
			SharedFeatures: []model.SharedFeature{},
		}},
		Warnings: []model.Warning{{
			Path:    "warning\rpath",
			Message: "message\u202Etxt",
		}},
	})
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	rendered := output.String()
	for _, control := range []string{"\x1b", "\r", "\u202e", "evil\npath.go"} {
		if strings.Contains(rendered, control) {
			t.Fatalf("text output contains unsafe sequence %q:\n%s", control, rendered)
		}
	}
	for _, escaped := range []string{"evil\\u000Apath.go", "name\\u001B", "warning\\u000Dpath"} {
		if !strings.Contains(rendered, escaped) {
			t.Errorf("text output missing %q:\n%s", escaped, rendered)
		}
	}
}

func TestTextDisclosesSuppressedGroups(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		SchemaVersion:           model.SchemaVersion,
		Threshold:               0.7,
		SuppressedLocationPairs: 15,
		SuppressedMatchGroups:   6,
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if !strings.Contains(output.String(), "15 location pair(s) suppressed through 6 baseline") {
		t.Fatalf("output does not disclose suppression identity scope:\n%s", output.String())
	}
}

func TestTextAnnotatesFocusedGroups(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		SchemaVersion:           model.SchemaVersion,
		Threshold:               0.7,
		TotalFocusedMatchGroups: 1,
		Configuration: model.EffectiveConfig{Focus: &model.FocusConfig{
			Mode: "explicit", DiscoveredFocusFiles: 1,
		}},
		Groups: []model.MatchGroup{{
			ID: "group", Similarity: 1, Focused: true, FocusedCount: 2,
		}},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	for _, expected := range []string{"1 focused group(s); 1 focused file(s)", "focused (2 occurrence(s))"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output missing %q:\n%s", expected, output.String())
		}
	}
}

func TestTextDisclosesLiteralDriftWithoutValues(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		SchemaVersion:      model.SchemaVersion,
		Threshold:          1,
		TotalMatchGroups:   1,
		TotalLocationPairs: 2,
		Groups: []model.MatchGroup{{
			ID: "group", Similarity: 1, LocationPairs: 2,
			LiteralEvidence: &model.LiteralEvidence{
				ComparedPairs: 2, PairsWithDifferences: 1, MaxDifferingPositions: 3,
			},
		}},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	rendered := output.String()
	if !strings.Contains(rendered, "1 of 2 location pair(s) differ at up to 3 position(s); values omitted") {
		t.Fatalf("output does not disclose source-free literal drift:\n%s", rendered)
	}
}

func TestTextExplainsWeightedDifferencesAndNormalizedIdentity(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		SchemaVersion: model.SchemaVersion,
		Threshold:     0.7,
		Groups: []model.MatchGroup{
			{
				ID: "different", Similarity: 0.75, LocationPairs: 1,
				Evidence: model.StructuralEvidence{
					Intersection: 6,
					Union:        8,
					LeftOnly: model.ProfileDifference{
						Fingerprint: "aaa", Total: 1,
						Features: []model.SharedFeature{{Feature: "node:left", Count: 1}},
					},
					RightOnly: model.ProfileDifference{
						Fingerprint: "bbb", Total: 1,
						Features: []model.SharedFeature{{Feature: "node:right", Count: 1}},
					},
				},
			},
			{
				ID: "identity", Similarity: 1, LocationPairs: 1,
				Evidence: model.StructuralEvidence{Intersection: 10, Union: 10},
			},
		},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	rendered := output.String()
	for _, expected := range []string{
		"weighted feature evidence: 6 intersection / 8 union",
		"A-only weighted units: 1 (aaa); top features: node:left ×1",
		"B-only weighted units: 1 (bbb); top features: node:right ×1",
		"100.0% normalized feature identity",
		"not proof of semantic or behavioral equivalence",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("output missing %q:\n%s", expected, rendered)
		}
	}
}

func TestTextShowsNestedBoundaryAndDiagnostics(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		SchemaVersion:      model.SchemaVersion,
		Threshold:          0.7,
		TotalMatchGroups:   1,
		TotalLocationPairs: 1,
		Groups: []model.MatchGroup{{
			ID:            "group",
			Similarity:    1,
			LocationPairs: 1,
			Profiles: []model.FragmentProfile{{
				Fingerprint:     "profile",
				OccurrenceCount: 1,
				Occurrences: []model.FragmentSummary{{
					Location: model.Location{
						Path: "file.ts", Language: "typescript", LanguageFamily: "typescript",
						Name: "outer", StartLine: 1, EndLine: 8,
					},
					NestedCount: 1,
				}},
			}},
		}},
		Warnings: []model.Warning{{
			Kind: "parse", Path: "file.ts", Message: "parse error", SkippedFragments: 2,
			Diagnostics: []model.ParseDiagnostic{{
				NodeKind: "ERROR", StartLine: 3, StartColumn: 4, EndLine: 3, EndColumn: 5,
			}},
		}},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	for _, expected := range []string{
		"function bodies are excluded from this score",
		"outer body only",
		"ERROR at 3:4-3:5",
		"2 comparison fragment(s) containing parse errors skipped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output missing %q:\n%s", expected, output.String())
		}
	}
}

func TestFormatFragmentDistinguishesShellTopLevelBody(t *testing.T) {
	t.Parallel()

	rendered := formatFragment(model.FragmentSummary{
		Location: model.Location{
			Path: "script.zsh", Language: "zsh", LanguageFamily: "shell",
			ComparisonDomain: "code", FragmentKind: "script", Name: "top-level",
			StartLine: 1, EndLine: 20,
		},
		NestedCount: 2,
	})
	if !strings.Contains(rendered, "top-level body only, 2 function(s) evaluated separately") {
		t.Fatalf("script fragment = %q", rendered)
	}
}

func TestTextDisclosesPerFileCoverage(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		SchemaVersion: model.SchemaVersion,
		Threshold:     0.85,
		Files:         2,
		Fragments:     3,
		FileCoverage: []model.FileCoverage{
			{Path: "functions.go", Language: "go", Status: "analyzed", FragmentCount: 3},
			{Path: "script.zsh", Language: "zsh", Status: "analyzed", ZeroReason: "no_boundaries"},
			{Path: "sqlc.go", Language: "go", Status: "excluded_generated", Generated: true},
		},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	rendered := output.String()
	for _, expected := range []string{
		"coverage: 1/2 analyzed file(s)",
		"files without comparison fragments (1):",
		"script.zsh [zsh; no_boundaries]",
		"generated sources: 0 analyzed, 1 excluded",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("output missing %q:\n%s", expected, rendered)
		}
	}
}

func TestTextDisclosesBaselineProfileEvidence(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		Configuration: model.EffectiveConfig{
			ScanProfileDigest: "active-digest",
			BaselineDigest:    "stored-digest",
			BaselineStatus:    "compatible",
		},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if !strings.Contains(
		output.String(),
		"baseline profile: compatible (stored-digest); active scan profile active-digest",
	) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestTextExplainsReviewRanking(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Text(&output, model.Report{
		Threshold: 0.85,
		Groups: []model.MatchGroup{{
			ID: "group", Similarity: 1, ReviewPriority: 9,
			ReviewSignals: []string{"same-name-cross-directory", "cross-directory"},
		}},
		Configuration: model.EffectiveConfig{Ranking: "review"},
	}); err != nil {
		t.Fatalf("Text: %v", err)
	}
	rendered := output.String()
	for _, expected := range []string{
		"ranking: explainable review signals before structural score",
		"review priority 9 · same-name-cross-directory, cross-directory",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("output missing %q:\n%s", expected, rendered)
		}
	}
}
