package parser

import (
	"context"
	goparser "go/parser"
	"go/token"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

// Exercise extraction and normalization, not merely recognition of the syntax.
// Valid neighbors survive, while errors in another function remain visible.
func TestModernSyntaxPreservesLocationsAndInvalidExclusion(t *testing.T) {
	for _, tc := range []struct {
		ext, content, name string
		line               int
	}{
		{"go", "package p\nfunc value(n int) *int { return new(max(1, n)) }\nfunc broken(n int) int { return n + }\n", "value", 2},
		{"ts", "export class Box<out T> {\nvalue(): T { return this.item; }\nbroken(): T { return + ; }\n}", "value", 2},
		{"tsx", "export class Box<out T> {\nvalue(): T { return this.item; }\nbroken(): T { return + ; }\n}", "value", 2},
		{"swift", "func value(_ n: Int, _ k: Int) -> Int {\nswitch k {\ncase (n / 2 + 1)...: return n\ndefault: return k\n}\n}\nfunc broken(_ n: Int) -> Int { return n + }\n", "value", 1},
	} {
		t.Run(tc.ext, func(t *testing.T) {
			spec, _ := language.Detect("input." + tc.ext)
			fragments, warnings, coverage := FileWithCoverage(context.Background(), source.File{DisplayPath: "input." + tc.ext, Language: spec, Content: []byte(tc.content)}, Options{MinTokens: 1})
			if len(fragments) != 1 || fragments[0].Location.Name != tc.name || fragments[0].Location.StartLine != tc.line {
				t.Fatalf("fragments=%+v", fragments)
			}
			if len(warnings) != 1 || warnings[0].TotalDiagnostics == 0 || warnings[0].SkippedFragments != 1 || coverage.CandidateFragments != 2 {
				t.Fatalf("warnings=%+v coverage=%+v", warnings, coverage)
			}
		})
	}
}

func TestGoAllocationSyntaxMatchesStandardParser(t *testing.T) {
	for _, tc := range []struct {
		body    string
		invalid bool
	}{
		{"return new(max(1, n))", false},
		{"return new(n + 1)", false},
		{"return new([]int)", false},
		{"return new(struct{ Value int })", false},
		{"return new(n + )", true},
		{"return new(max(1, n)", true},
	} {
		t.Run(tc.body, func(t *testing.T) {
			content := "package p\nfunc value(n int) any { " + tc.body + " }\n"
			_, err := goparser.ParseFile(token.NewFileSet(), "input.go", content, goparser.AllErrors)
			if (err != nil) != tc.invalid {
				t.Fatalf("standard parser error=%v invalid=%v", err, tc.invalid)
			}
			spec, _ := language.Detect("input.go")
			fragments, warnings := FileWithOptions(context.Background(), source.File{DisplayPath: "input.go", Language: spec, Content: []byte(content)}, Options{MinTokens: 1})
			if (len(warnings) > 0) != tc.invalid || (len(fragments) == 0) != tc.invalid {
				t.Fatalf("fragments=%+v warnings=%+v", fragments, warnings)
			}
		})
	}
}
