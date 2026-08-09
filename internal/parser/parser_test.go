package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

func TestFileExtractsNamedAndAnonymousFunctions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "functions.js")
	content := `
export function named(value) {
  const clean = value.trim();
  return clean.includes("@") && clean.includes(".");
}

export const other = (input) => {
  const clean = input.trim();
  return clean.includes("@") && clean.includes(".");
};
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("JavaScript grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "functions.js",
		Language:    spec,
	}, 10)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(fragments) != 2 {
		t.Fatalf("fragments = %d, want 2", len(fragments))
	}
	if fragments[0].Location.Name != "named" {
		t.Errorf("first name = %q, want named", fragments[0].Location.Name)
	}
	if fragments[1].Location.Name != "other" {
		t.Errorf("second name = %q, want other", fragments[1].Location.Name)
	}
}

func TestFileWarnsAndSkipsInvalidFragment(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.py")
	if err := os.WriteFile(path, []byte("def broken(:\n  return True\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Python grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "broken.py",
		Language:    spec,
	}, 1)
	if len(fragments) != 0 {
		t.Fatalf("fragments = %d, want none", len(fragments))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one parse warning", warnings)
	}
	if warnings[0].Kind != "parse" || warnings[0].Language != "python" ||
		warnings[0].TotalDiagnostics == 0 || len(warnings[0].Diagnostics) == 0 ||
		warnings[0].Diagnostics[0].StartLine < 1 {
		t.Fatalf("parse warning is not actionable: %#v", warnings[0])
	}
}

func TestFileRepairsKnownJSXRawAmpersandGrammarGap(t *testing.T) {
	t.Parallel()

	for _, extension := range []string{".jsx", ".tsx"} {
		extension := extension
		t.Run(extension, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "component"+extension)
			content := `export function Header() {
  return <h1>Roles & permissions</h1>;
}
`
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{
				Path: path, DisplayPath: "component" + extension, Language: spec,
			}, 1)
			if len(warnings) != 0 || len(fragments) != 1 {
				t.Fatalf("fragments/warnings = %#v/%#v, want one/none", fragments, warnings)
			}
		})
	}
}

func TestFileDoesNotRepairInvalidJSXExpression(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.tsx")
	content := `export function Header(value: number) {
  return <h1>{value & }</h1>;
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "broken.tsx", Language: spec,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "parse" ||
		warnings[0].SkippedFragments != 1 {
		t.Fatalf("fragments/warnings = %#v/%#v, want skipped invalid function", fragments, warnings)
	}
}

func TestFileAnnotatesNestedFunctionBoundaries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested.ts")
	content := `
export function outer(value: string) {
  const inner = () => value.trim();
  return inner();
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("TypeScript grammar not detected")
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "nested.ts", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 2 {
		t.Fatalf("fragments/warnings = %#v/%#v, want two/none", fragments, warnings)
	}
	if fragments[0].NestedCount != 1 {
		t.Fatalf("outer nested count = %d, want 1", fragments[0].NestedCount)
	}
	if fragments[1].NestingDepth != 1 || fragments[1].Parent == nil ||
		fragments[1].ParentID != fragments[0].Fingerprint {
		t.Fatalf("nested metadata = %#v, want parent linkage", fragments[1])
	}
}

func TestFileExtractsOnlyTopLevelSQLQueries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "queries.sql")
	content := `-- name: ListVisible :many
WITH visible AS (
  SELECT id FROM folders WHERE tenant_id = ?
)
SELECT id FROM visible ORDER BY id;

CREATE TABLE ignored (id INTEGER PRIMARY KEY);

UPDATE folders SET name = $1 WHERE id = $2 AND tenant_id = $3;
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("SQL grammar not detected")
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "queries.sql", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 2 {
		t.Fatalf("fragments/warnings = %#v/%#v, want two query fragments and no warnings", fragments, warnings)
	}
	for _, fragment := range fragments {
		if fragment.Location.Language != "sql" || fragment.Location.LanguageFamily != "sql" ||
			fragment.Location.ComparisonDomain != "sql-query" || fragment.Location.FragmentKind != "query" {
			t.Fatalf("SQL location = %#v", fragment.Location)
		}
		if fragment.NestingDepth != 0 || fragment.Parent != nil {
			t.Fatalf("SQL query was treated as nested: %#v", fragment)
		}
	}
	if fragments[0].Location.Name != "ListVisible" || fragments[1].Location.Name != "query@9" {
		t.Fatalf("SQL names = %q/%q", fragments[0].Location.Name, fragments[1].Location.Name)
	}
}

func TestSQLCNameMustBeExactAndImmediatelyPrecedeQuery(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "names.sql")
	content := `-- name: ExactName :many
SELECT id FROM users;
-- name: Separated :one

SELECT id FROM members;
-- name: invalid-name :one
SELECT id FROM guests;
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "names.sql", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 3 {
		t.Fatalf("fragments/warnings = %#v/%#v", fragments, warnings)
	}
	if fragments[0].Location.Name != "ExactName" || fragments[1].Location.Name != "query@5" ||
		fragments[2].Location.Name != "query@7" {
		t.Fatalf("names = %q/%q/%q", fragments[0].Location.Name, fragments[1].Location.Name, fragments[2].Location.Name)
	}
}

func TestSQLParseErrorDoesNotHideSeparateValidQuery(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "partial.sql")
	content := "SELECT id FROM broken WHERE tenant_id = @@@;\nSELECT id FROM valid WHERE tenant_id = $1;\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "partial.sql", Language: spec,
	}, 1)
	if len(fragments) != 1 || fragments[0].Location.StartLine != 2 {
		t.Fatalf("fragments = %#v, want only second query", fragments)
	}
	if len(warnings) != 1 || warnings[0].Kind != "parse" || warnings[0].TotalDiagnostics == 0 ||
		warnings[0].SkippedFragments == 0 {
		t.Fatalf("warnings = %#v, want visible skipped invalid query", warnings)
	}
}

func TestSQLDDLParseWarningDoesNotClaimFragmentsWereSkipped(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "schema.sql")
	content := `PRAGMA foreign_keys = ON;
CREATE TABLE documents (
  metadata TEXT CHECK(json_valid(metadata))
);
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "schema.sql", Language: spec,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "parse" ||
		warnings[0].TotalDiagnostics == 0 || warnings[0].SkippedFragments != 0 {
		t.Fatalf("fragments/warnings = %#v/%#v", fragments, warnings)
	}
	if warnings[0].Message != "syntax tree contains parse errors; comparison coverage may be incomplete" {
		t.Fatalf("warning message = %q", warnings[0].Message)
	}
}

func TestFileRepairsSQLiteAndSQLCQueryForms(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sqlite.sql")
	content := `-- name: PageItems :many
SELECT * FROM items LIMIT ? OFFSET ?;
-- name: SearchItems :many
SELECT * FROM items LIMIT sqlc.arg(limit) OFFSET sqlc.narg('offset');
-- name: UpsertItem :exec
INSERT INTO items (id, name) VALUES (?, ?)
ON CONFLICT (id) DO UPDATE SET name = excluded.name;
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "sqlite.sql", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 3 {
		t.Fatalf("fragments/warnings = %#v/%#v, want three/none", fragments, warnings)
	}
	for index, name := range []string{"PageItems", "SearchItems", "UpsertItem"} {
		if fragments[index].Location.Name != name {
			t.Errorf("fragment %d name = %q, want %q", index, fragments[index].Location.Name, name)
		}
	}
	if fragments[0].Features["node:query:limit"] == 0 ||
		fragments[0].Features["node:query:offset"] == 0 ||
		fragments[2].Features["node:query:conflict"] == 0 {
		t.Fatalf("repaired structural features = %#v / %#v", fragments[0].Features, fragments[2].Features)
	}
}

func TestFileDoesNotRepairMalformedSQLiteForms(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.sql")
	content := `SELECT * FROM items LIMIT ? OFFSET;
INSERT INTO items (id) VALUES (?) ON CONFLICT (id,) DO NOTHING;
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	_, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "broken.sql", Language: spec,
	}, 1)
	if len(warnings) != 1 || warnings[0].Kind != "parse" || warnings[0].TotalDiagnostics == 0 {
		t.Fatalf("warnings = %#v, want visible malformed syntax", warnings)
	}
}

func TestFileEnforcesReadLimit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "large.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Example() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Go grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "large.go",
		Language:    spec,
		MaxBytes:    8,
	}, 1)
	if len(fragments) != 0 {
		t.Fatalf("fragments = %d, want none", len(fragments))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one size warning", warnings)
	}
}

func TestFileRejectsIdentityChangeAfterDiscovery(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "changed.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc First() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(first): %v", err)
	}
	originalInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	replacementPath := filepath.Join(filepath.Dir(path), "replacement.go")
	if err := os.WriteFile(replacementPath, []byte("package sample\nfunc Second() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(replacement): %v", err)
	}
	replacementInfo, err := os.Stat(replacementPath)
	if err != nil {
		t.Fatalf("Stat(replacement): %v", err)
	}
	if os.SameFile(originalInfo, replacementInfo) {
		t.Fatal("simultaneous fixture files unexpectedly share an identity")
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.Rename(replacementPath, path); err != nil {
		t.Fatalf("Rename(replacement): %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Go grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "changed.go",
		Language:    spec,
		Info:        originalInfo,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 {
		t.Fatalf("fragments/warnings = %d/%#v, want 0/one", len(fragments), warnings)
	}
	if strings.Contains(warnings[0].Message, path) {
		t.Fatalf("warning message %q contains absolute path", warnings[0].Message)
	}
}

func TestFileHandlesDeepSyntaxWithoutRecursiveWalk(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "deep.js")
	content := "function deep(value) { return " +
		strings.Repeat("(", 10_000) +
		"value" +
		strings.Repeat(")", 10_000) +
		"; }\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("JavaScript grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "deep.js",
		Language:    spec,
	}, 1)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(fragments) != 1 {
		t.Fatalf("fragments = %d, want one", len(fragments))
	}
}

func TestTypeScriptDialectsExtractFunctions(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"typed.ts": `
export function present(value: string): boolean {
  return value.length > 0;
}
`,
		"view.tsx": `
export const View = () => {
  return <div>hello</div>;
};
`,
	}
	for name, content := range tests {
		name := name
		content := content
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, ok := language.Detect(path)
			if !ok {
				t.Fatalf("%s grammar not detected", name)
			}
			fragments, warnings := File(context.Background(), source.File{
				Path:        path,
				DisplayPath: name,
				Language:    spec,
			}, 1)
			if len(warnings) != 0 {
				t.Fatalf("warnings = %#v", warnings)
			}
			if len(fragments) != 1 {
				t.Fatalf("fragments = %d, want 1", len(fragments))
			}
		})
	}
}
