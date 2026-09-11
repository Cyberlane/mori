package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
	ts "github.com/tree-sitter/go-tree-sitter"
)

// These original minimal examples isolate valid constructs encountered during
// public-repository review. They exercise the actual grammar without repairs.
func TestPublicRepositoryGrammarRegressions(t *testing.T) {
	cases := []struct {
		name, ext, content string
		invalid            bool
	}{
		{"go_new_expression", "go", "package p; func makeValue(n int) *int { return new(max(1, n)) }", false},
		{"go_new_arithmetic", "go", "package p; func makeValue(n int) *int { return new(n + 1) }", false},
		{"go_new_type", "go", "package p; func makeValue() *int { return new(int) }", false},
		{"go_make_type", "go", "package p; func makeValue(n int) []int { return make([]int, n) }", false},
		{"go_new_broken", "go", "package p; func makeValue(n int) *int { return new(n + ) }", true},
		{"variance_out", "ts", "export class Box<out T extends Node = Node> { get(): T { return this.value; } }", false},
		{"variance_in", "ts", "interface Setter<in T> { set(value: T): void; }", false},
		{"variance_in_out", "ts", "type State<in out T> = { get(): T; set(value: T): void };", false},
		{"variance_comment", "ts", "class Box<out /* note */ T> { get(): T { return this.value; } }", false},
		{"variance_identifier", "ts", "class Box<out> { get(): out { return this.value; } }", false},
		{"variance_missing_constraint", "ts", "class Box<out T extends > {}", true},
		{"variance_broken_body", "ts", "class Box<out T> { get(): T { return + ; } }", true},
		{"swift_literal_partial_range", "swift", "func count(_ n: Int) -> Int { switch n {\ncase 2...: return 1\ndefault: return 0\n} }", false},
		{"swift_closed_expression_range", "swift", "func count(_ n: Int, _ k: Int) -> Int { switch k {\ncase (n + 1)...(n + 3): return 1\ndefault: return 0\n} }", false},
		{"swift_tuple_binding", "swift", "func count(_ value: (Int, Int)) -> Int { switch value {\ncase let (a, b): return a + b\n} }", false},
		{"swift_tuple_expression", "swift", "func pair(_ n: Int) -> (Int, Int) { return (n + 1, n * 2) }", false},
		{"swift_broken_partial_range", "swift", "func count(_ n: Int) -> Int { switch n {\ncase (n + )...: return 1\ndefault: return 0\n} }", true},
		{"swift_broken_tuple", "swift", "func pair(_ n: Int) -> (Int, Int) { return (n + , n * 2) }", true},
		{"swift_expression_partial_range", "swift", "func count(_ n: Int, _ k: Int) -> Int { switch k {\ncase (n / 2 + 1)...: return n\ndefault: return k\n} }", false},
		{"overloads", "ts", "interface Factory {\n <A>(a: A): A\n <B>(b: B): B\n}", false},
		{"overloads_comments_crlf", "ts", "interface Factory {\r\n <Δ>(a: Δ): Δ\r\n /* next */ <B extends Array<string>>(b: B): B\r\n}", false},
		{"overload_function_constraint", "ts", "interface F {\n<A>(a:A):A\n<B extends () => string>(b:B):B\n}", false},
		{"overload_literal_constraint", "ts", "interface F {\n<A>(a:A):A\n<B extends \">\">(b:B):B\n}", false},
		{"overload_comment_constraint", "ts", "interface F {\n<A>(a:A):A\n<B /* > */ extends Array<string>>(b:B):B\n}", false},
		{"overloads_semicolons", "ts", "interface Factory { <A>(a: A): A; <B>(b: B): B; }", false},
		{"multiline_generic_alias", "ts", "type Values = Array\n<string>;", false},
		{"multiline_generic_return", "ts", "interface F { (): Array\n<string>; }", false},
		{"import_array", "ts", "interface X { scriptAst?: import('@babel/types').Statement[]; }", false},
		{"import_nested", "ts", "interface X { node: import(/* module */ 'pkg').Nested.Node<Array<string>>; }", false},
		{"import_function", "ts", "function keep(x: import('pkg').Node) { return x; }", false},
		{"typeof_import_call", "ts", "function keep() { return original<typeof import('./module')>(); }", false},
		{"keyword_property_newlines", "ts", "interface X {\n first?: string | undefined\n in2?: number | undefined\n in?: string | undefined\n}", false},
		{"keyword_property", "ts", "interface X { in?: string; in2?: string; return: string; }", false},
		{"expression_less", "ts", "function keep(a:number,b:number) { return a\n <b; }", false},
		{"missing_signature_operand", "ts", "interface X { <A>(a: A): ; }", true},
		{"missing_import_module", "ts", "interface X { x: import().Node; }", true},
		{"missing_import_member", "ts", "interface X { x: import('x').; }", true},
		{"missing_generic_delimiter", "ts", "function keep() { return original<typeof import('x')>(; }", true},
		{"keyword_exports", "js", "const value = 1; export { value as if, value as const, value as return, value as this, value as super, value as yield };", false},
		{"keyword_exports_comment", "js", "const Δ = 1; export { Δ /* x */ as /* y */ while };", false},
		{"missing_export_alias", "js", "export { value as };", true},
		{"annotated_varargs", "java", "class X { boolean keep(Class<?> @Nullable ... types) { return false; } }", false},
		{"annotated_varargs_qualified", "java", "class X { boolean keep(Class<?> @pkg.Nullable(\"x\") /* x */ @Other ... types) { return false; } }", false},
		{"ordinary_varargs", "java", "class X { boolean keep(Class<?> ... types) { return false; } }", false},
		{"misplaced_annotation", "java", "class X { boolean keep(Class<?> ... @Nullable types) { return false; } }", true},
		{"missing_varargs_type", "java", "class X { boolean keep(@Nullable ... types) { return false; } }", true},
	}
	for _, c := range cases {
		extensions := []string{c.ext}
		if c.ext == "ts" {
			extensions = append(extensions, "tsx")
		}
		for _, ext := range extensions {
			t.Run(c.name+"_"+ext, func(t *testing.T) {
				spec, _ := language.Detect("input." + ext)
				p := ts.NewParser()
				defer p.Close()
				if err := p.SetLanguage(spec.NewLanguage()); err != nil {
					t.Fatal(err)
				}
				tree := p.Parse([]byte(c.content), nil)
				defer tree.Close()
				if tree.RootNode().HasError() != c.invalid {
					t.Fatalf("invalid=%v tree=%s", c.invalid, tree.RootNode().ToSexp())
				}
			})
		}
	}
}

func TestTypeOnlyGrammarRepairsPreserveNeighborLocation(t *testing.T) {
	for _, ext := range []string{"ts", "tsx"} {
		t.Run(ext, func(t *testing.T) {
			content := "interface Factory {\n <A>(a:A):A\n <B>(b:B):B\n value: import('x').Node[];\n}\nfunction keep(x:number) { return x*x+x; }\nfunction broken(x:number) { return x + ; }\n"
			path := filepath.Join(t.TempDir(), "input."+ext)
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings, coverage := FileWithCoverage(context.Background(), source.File{Path: path, DisplayPath: "input." + ext, Language: spec}, Options{MinTokens: 1})
			if len(fragments) != 1 || fragments[0].Location.Name != "keep" || fragments[0].Location.StartLine != 6 {
				t.Fatalf("fragments=%+v", fragments)
			}
			if len(warnings) != 1 || warnings[0].SkippedFragments != 1 || coverage.CandidateFragments != 2 {
				t.Fatalf("warnings=%+v coverage=%+v", warnings, coverage)
			}
			if !strings.Contains(warnings[0].Message, "parse") {
				t.Fatalf("warning=%+v", warnings)
			}
		})
	}
}
