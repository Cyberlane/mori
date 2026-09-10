package report

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestAgentExplainsReviewCoverageAndRanking(t *testing.T) {
	t.Parallel()
	value := model.Report{
		Review:        &model.ReviewOutcome{Policy: "advisory", Status: "findings", Analysis: "incomplete", Findings: 1},
		Configuration: model.EffectiveConfig{Focus: &model.FocusConfig{PathEvidence: []model.FocusPathEvidence{{Path: "changed.ts", Status: "analyzed", ChangedLines: []model.LineInterval{{StartLine: 20, EndLine: 25}}}}}},
		Warnings: []model.Warning{
			{Kind: "parse", Path: "./changed.ts", Message: "outside changed lines", TotalDiagnostics: 2, Diagnostics: []model.ParseDiagnostic{{StartLine: 1, EndLine: 1}}},
			{Kind: "parse", Path: "existing.sql", Message: "dialect gap", TotalDiagnostics: 3},
			{Kind: "focus", Message: "excluded source"},
		},
		FileCoverage: []model.FileCoverage{
			{Path: "existing.sql", Status: "analyzed", ZeroReason: "invalid_fragments"},
			{Path: "types.ts", Status: "analyzed", ZeroReason: "no_boundaries"},
			{Path: "tiny.ts", Status: "analyzed", ZeroReason: "below_token_floor"},
			{Path: "generated.ts", Status: "excluded_generated"},
		},
		TotalFocusedMatchGroups: 1,
		Groups:                  []model.MatchGroup{{ID: "pair", Focused: true, ReviewPriority: 15, ReviewSignals: []string{"branching", "same-name"}}},
	}
	var output bytes.Buffer
	if err := Agent(&output, value); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"review: policy advisory; status findings; analysis incomplete; coverage policy met false; 1 finding(s); acknowledged false",
		"diagnostic scope [focused file]: 1 warning(s); 2 parse diagnostic(s)",
		"diagnostic scope [background file]: 1 warning(s); 3 parse diagnostic(s)",
		"diagnostic scope [unscoped]: 1 warning(s); 0 parse diagnostic(s)",
		"warning[parse] ./changed.ts (focused file): outside changed lines",
		"review priority: 15; signals: branching, same-name",
		"no comparison fragments: 1 file(s); no comparable function/query boundaries",
		"no comparison fragments: 1 file(s); all candidates below token floor",
		"no comparison fragments: 1 file(s); parser diagnostics or invalid fragments",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "reason unavailable") {
		t.Fatalf("generated exclusion counted as analyzed: %s", output.String())
	}
}

func TestAgentWarningScopeDoesNotInferFocusWithoutEvidence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		path  string
		focus *model.FocusConfig
		want  string
	}{
		{"source.go", nil, "unscoped"},
		{"", &model.FocusConfig{}, "unscoped"},
		{"other.go", &model.FocusConfig{PathEvidence: []model.FocusPathEvidence{{Path: "source.go"}}}, "background file"},
	} {
		if got := agentWarningScope(test.path, test.focus); got != test.want {
			t.Fatalf("scope = %q, want %q", got, test.want)
		}
	}
}

func TestAgentPrioritySignalsRemainBoundedAndEscaped(t *testing.T) {
	t.Parallel()
	signals := []string{"first\x1b[2J", "second", "third", "fourth", "fifth", "sixth", "hidden"}
	var output bytes.Buffer
	if err := Agent(&output, model.Report{TotalMatchGroups: 1, Groups: []model.MatchGroup{{ID: "pair", ReviewSignals: signals}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b") || strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "1 more in complete JSON") {
		t.Fatalf("unsafe or unbounded priority output: %q", output.String())
	}
}

func BenchmarkAgentBoundedSummary(b *testing.B) {
	value := model.Report{TotalMatchGroups: 1000}
	for index := 0; index < 1000; index++ {
		value.Groups = append(value.Groups, model.MatchGroup{ID: fmt.Sprintf("pair-%d", index), ReviewSignals: []string{"branching"}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if err := Agent(io.Discard, value); err != nil {
			b.Fatal(err)
		}
	}
}
