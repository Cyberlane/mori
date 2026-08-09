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
		"outer body only",
		"ERROR at 3:4-3:5",
		"2 comparison fragment(s) containing parse errors skipped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output missing %q:\n%s", expected, output.String())
		}
	}
}
