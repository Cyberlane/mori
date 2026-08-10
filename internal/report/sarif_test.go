package report

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/model"
)

func TestSARIFUsesStableRulesLocationsAndSuppressionCounts(t *testing.T) {
	t.Parallel()
	report := model.Report{
		SchemaVersion:  model.SchemaVersion,
		Tool:           buildinfo.Info{Version: "test", NormalizationVersion: 9},
		CandidatePairs: 4, TotalMatchGroups: 2, SuppressedMatchGroups: 1,
		SuppressedLocationPairs: 3, Truncated: true,
		Configuration: model.EffectiveConfig{
			ScanProfileDigest: "digest", StdinPath: "sample.go",
		},
		Groups: []model.MatchGroup{{
			ID: "left:right", Similarity: 1, LocationPairs: 1, Focused: true,
			FocusedCount: 1, Evidence: model.StructuralEvidence{Intersection: 20, Union: 20},
			Profiles: []model.FragmentProfile{
				{Occurrences: []model.FragmentSummary{
					{Location: model.Location{
						Path: "source dir/a.go", Language: "go", FragmentKind: "function", Name: "First",
						StartLine: 2, EndLine: 4,
					}},
				}},
				{Occurrences: []model.FragmentSummary{
					{Location: model.Location{
						Path: "b.go", Language: "go", FragmentKind: "function", Name: "Second",
						StartLine: 6, EndLine: 9,
					}},
				}},
			},
		}},
		Warnings: []model.Warning{{
			Kind: "parse", Path: "broken.go", Language: "go", Message: "parse incomplete",
			TotalDiagnostics: 1, Diagnostics: []model.ParseDiagnostic{{
				NodeKind: "ERROR", StartLine: 3, StartColumn: 4, EndLine: 3, EndColumn: 7,
			}},
		}},
	}

	var first bytes.Buffer
	if err := SARIF(&first, report); err != nil {
		t.Fatalf("SARIF: %v", err)
	}
	var second bytes.Buffer
	if err := SARIF(&second, report); err != nil {
		t.Fatalf("SARIF again: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("SARIF output is not deterministic")
	}

	var decoded map[string]any
	if err := json.Unmarshal(first.Bytes(), &decoded); err != nil {
		t.Fatalf("decode SARIF: %v", err)
	}
	runs := decoded["runs"].([]any)
	run := runs[0].(map[string]any)
	results := run["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("results = %d, want similarity plus parse warning", len(results))
	}
	firstResult := results[0].(map[string]any)
	if firstResult["ruleId"] != similarityRuleID ||
		len(firstResult["relatedLocations"].([]any)) != 1 {
		t.Fatalf("similarity result = %#v", firstResult)
	}
	if _, exists := firstResult["baselineState"]; exists {
		t.Fatalf("similarity result asserts unsupported baseline state: %#v", firstResult)
	}
	locationURI := firstResult["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)["artifactLocation"].(map[string]any)["uri"]
	if locationURI != "source%20dir/a.go" {
		t.Fatalf("artifact URI = %#v", locationURI)
	}
	message := firstResult["message"].(map[string]any)["text"].(string)
	if !strings.Contains(message, "normalized feature identity") ||
		!strings.Contains(message, "does not prove semantic or behavioral equivalence") {
		t.Fatalf("similarity message = %q", message)
	}
	warningResult := results[1].(map[string]any)
	if warningResult["ruleId"] != parseWarningRuleID {
		t.Fatalf("warning result = %#v", warningResult)
	}
	region := warningResult["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)["region"].(map[string]any)
	if !reflect.DeepEqual(region, map[string]any{
		"startLine": float64(3), "startColumn": float64(4),
		"endLine": float64(3), "endColumn": float64(7),
	}) {
		t.Fatalf("warning region = %#v", region)
	}
	invocation := run["invocations"].([]any)[0].(map[string]any)
	properties := invocation["properties"].(map[string]any)
	if properties["suppressedMatchGroups"] != float64(1) || properties["truncated"] != true {
		t.Fatalf("invocation properties = %#v", properties)
	}
}
