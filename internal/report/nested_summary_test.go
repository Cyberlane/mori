package report

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestConciseReportsExposeNestedScoringBoundary(t *testing.T) {
	for name, render := range map[string]func(io.Writer, model.Report) error{"agent": Agent, "compact": Compact} {
		t.Run(name, func(t *testing.T) {
			value := model.Report{TotalMatchGroups: 1, Groups: []model.MatchGroup{{ID: "nested", Profiles: []model.FragmentProfile{{Occurrences: []model.FragmentSummary{{NestedCount: 2, Location: model.Location{Path: "one.go", Name: "aggregate"}}}}}}}}
			var out bytes.Buffer
			if err := render(&out, value); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "outer body only") || !strings.Contains(out.String(), "nested functions evaluated separately") {
				t.Fatalf("missing boundary: %s", out.String())
			}
			value.Groups[0].Profiles[0].Occurrences[0].NestedCount = 0
			out.Reset()
			if err := render(&out, value); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "outer body only") {
				t.Fatal("invented boundary for non-nested body")
			}
		})
	}
}

func TestHumanReportsDiscloseClassificationOverrides(t *testing.T) {
	for name, render := range map[string]func(io.Writer, model.Report) error{"agent": Agent, "compact": Compact, "text": Text} {
		t.Run(name, func(t *testing.T) {
			value := model.Report{Configuration: model.EffectiveConfig{ProductionPaths: []string{"private/path"}, TestPaths: []string{"private/tests", "private/integration"}}}
			var out bytes.Buffer
			if err := render(&out, value); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "classification overrides: 1 production path(s), 2 test path(s)") || !strings.Contains(out.String(), "--format json for exact paths") {
				t.Fatalf("missing policy: %s", out.String())
			}
			if strings.Contains(out.String(), "private/") {
				t.Fatal("summary included raw paths")
			}
			value.Configuration = model.EffectiveConfig{}
			out.Reset()
			if err := render(&out, value); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "classification overrides:") {
				t.Fatal("invented overrides")
			}
		})
	}
}
