package report

import (
	"bytes"
	"github.com/Cyberlane/mori/internal/model"
	"io"
	"strings"
	"testing"
)

func TestConciseReportsDiscloseUnsupportedSource(t *testing.T) {
	t.Parallel()
	value := model.Report{FileCoverage: []model.FileCoverage{{Language: "swift"}}, Coverage: model.CoverageSummary{SupportedFiles: 1, AnalyzedFiles: 1, UnsupportedExtensions: []model.UnsupportedExtension{{Extension: ".zig", FileCount: 793}, {Extension: ".svelte", FileCount: 42}}}}
	for name, render := range map[string]func(io.Writer, model.Report) error{"agent": Agent, "compact": Compact, "text": Text} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := render(&out, value); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"swift=1", ".zig=793", ".svelte=42", "supported-file coverage is not repository coverage"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing %q: %s", want, out.String())
				}
			}
		})
	}
}
