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
		SchemaVersion: 1,
		Threshold:     0.7,
		Files:         2,
		Fragments:     2,
		TotalMatches:  1,
		Matches: []model.Match{{
			Similarity: 1,
			Left: model.FragmentSummary{Location: model.Location{
				Path:      "evil\npath.go",
				Language:  "go",
				Name:      "name\x1b[31m",
				StartLine: 1,
				EndLine:   2,
			}},
			Right: model.FragmentSummary{Location: model.Location{
				Path:      "safe.go",
				Language:  "go",
				Name:      "safe",
				StartLine: 1,
				EndLine:   2,
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
