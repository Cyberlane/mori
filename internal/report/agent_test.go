package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestAgentBoundsFocusedGroupsAndWarnings(t *testing.T) {
	t.Parallel()
	report := model.Report{
		TotalMatchGroups:        40,
		TotalFocusedMatchGroups: 30,
		TotalLocationPairs:      90,
		CandidatePairs:          1234,
		Configuration: model.EffectiveConfig{
			Focus: &model.FocusConfig{RequiredFocusFiles: 2, CoveredFocusFiles: 2},
			Input: &model.InputSnapshot{
				Mode: "git-index", HeadCommit: strings.Repeat("a", 40),
				IndexDigest: strings.Repeat("b", 64),
			},
		},
		Coverage: model.CoverageSummary{
			SupportedFiles: 3, AnalyzedFiles: 2, FragmentFiles: 2,
			WarningCount: 22, ParseDiagnosticCount: 1,
		},
	}
	for index := 0; index < 30; index++ {
		report.Groups = append(report.Groups, model.MatchGroup{
			ID: fmt.Sprintf("pair-%02d", index), Similarity: 0.91,
			LocationPairs: 1, Focused: true,
			Profiles: []model.FragmentProfile{
				{Occurrences: []model.FragmentSummary{{Location: model.Location{
					Path: "left.go", Name: "Left", StartLine: index + 1, EndLine: index + 2,
				}}}},
				{Occurrences: []model.FragmentSummary{{Location: model.Location{
					Path: "right.go", Name: "Right", StartLine: 9, EndLine: 9,
				}}}},
			},
		})
	}
	for index := 0; index < 22; index++ {
		report.Warnings = append(report.Warnings, model.Warning{
			Kind: "parse", Path: fmt.Sprintf("file-%02d.go", index), Message: "diagnostic",
		})
	}

	var output bytes.Buffer
	if err := Agent(&output, report); err != nil {
		t.Fatalf("Agent: %v", err)
	}
	rendered := output.String()
	for _, expected := range []string{
		"30 focused group(s)",
		"input: git-index; HEAD aaaaaaaaaaaa; index bbbbbbbbbbbb",
		"focus: 2/2",
		"left.go:1-2:Left <-> right.go:9:Right",
		"groups omitted from context: 5",
		"warnings omitted from context: 2",
		"do not prove behavioral equivalence",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("agent output missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "pair-25") || strings.Contains(rendered, "file-20.go") {
		t.Fatalf("agent output exceeded bounds:\n%s", rendered)
	}
}

func TestAgentUsesAllGroupsWithoutFocus(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := Agent(&output, model.Report{
		TotalMatchGroups: 1,
		Groups:           []model.MatchGroup{{ID: "pair", Similarity: 1}},
	}); err != nil {
		t.Fatalf("Agent: %v", err)
	}
	if !strings.Contains(output.String(), "1 group(s)") || !strings.Contains(output.String(), "1. 100.0% pair") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestAgentPrefersExactFocusedPathPair(t *testing.T) {
	t.Parallel()
	focus := &model.FocusConfig{PathEvidence: []model.FocusPathEvidence{{
		Path: "changed.go", Status: "analyzed",
		ChangedLines: []model.LineInterval{{StartLine: 40, EndLine: 40}},
	}}}
	group := model.MatchGroup{
		PathPairs: []model.LocationPair{
			{Left: model.Location{Path: "a.go", Name: "A", StartLine: 1, EndLine: 2}, Right: model.Location{Path: "b.go", Name: "B", StartLine: 3, EndLine: 4}},
			{Left: model.Location{Path: "changed.go", Name: "Changed", StartLine: 35, EndLine: 45}, Right: model.Location{Path: "existing.go", Name: "Existing", StartLine: 8, EndLine: 10}},
		},
	}
	got := agentGroupLocations(group, focus)
	if got != "changed.go:35-45:Changed <-> existing.go:8-10:Existing" {
		t.Fatalf("focused locations = %q", got)
	}
}
