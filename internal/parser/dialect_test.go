package parser

import (
	"context"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
	"strings"
	"testing"
)

func TestFlowDialectDiagnosticPreservesInvalidFragmentExclusion(t *testing.T) {
	for _, header := range []string{"// @flow\n", "/*\n * @flow strict-local\n */\n"} {
		content := header + "function typed(x: number): number { return x + 1; }\nfunction valid(x) { return x * x + 1; }\n"
		spec, _ := language.Detect("input.js")
		fragments, warnings, _ := FileWithCoverage(context.Background(), source.File{DisplayPath: "input.js", Language: spec, Content: []byte(content)}, Options{MinTokens: 1})
		if len(warnings) != 1 || warnings[0].Kind != "parse" || !strings.Contains(warnings[0].Message, "Flow") || warnings[0].TotalDiagnostics == 0 {
			t.Fatalf("warnings=%+v", warnings)
		}
		if len(fragments) != 1 || fragments[0].Location.Name != "valid" {
			t.Fatalf("fragments=%+v", fragments)
		}
	}
}

func TestFlowMarkerRequiresLeadingComment(t *testing.T) {
	for _, content := range []string{
		"const marker = '@flow'; function broken(x) { return x + ; }",
		"// @flowing\nfunction broken(x) { return x + ; }",
		"// @noflow\nfunction broken(x) { return x + ; }",
		"function broken(x) { /* @flow */ return x + ; }",
	} {
		spec, _ := language.Detect("input.js")
		_, warnings := FileWithOptions(context.Background(), source.File{DisplayPath: "input.js", Language: spec, Content: []byte(content)}, Options{MinTokens: 1})
		if len(warnings) != 1 || strings.Contains(warnings[0].Message, "Flow") {
			t.Fatalf("content=%s warnings=%+v", content, warnings)
		}
	}
}
