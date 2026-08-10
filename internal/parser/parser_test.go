package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
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

func TestShellDialectsExtractNamedFunctions(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"functions.sh": `first() { printf '%s\n' "$1"; }
function second { printf '%s\n' "$1"; }
`,
		"functions.zsh": `first() { print -r -- "$1"; }
function second { print -r -- "$1"; }
`,
	}
	for name, content := range tests {
		name, content := name, content
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
				Path: path, DisplayPath: name, Language: spec,
			}, 1)
			if len(warnings) != 0 || len(fragments) != 3 {
				t.Fatalf("fragments/warnings = %#v/%#v, want script plus two functions/none", fragments, warnings)
			}
			if fragments[0].Location.Name != "top-level" ||
				fragments[0].Location.FragmentKind != "script" || fragments[0].NestedCount != 2 {
				t.Fatalf("top-level fragment = %#v", fragments[0])
			}
			if fragments[1].Location.Name != "first" || fragments[2].Location.Name != "second" {
				t.Fatalf("function names = %q/%q, want first/second", fragments[1].Location.Name, fragments[2].Location.Name)
			}
			for _, fragment := range fragments[1:] {
				if fragment.NestingDepth != 1 || fragment.Parent == nil ||
					fragment.Parent.FragmentKind != "script" {
					t.Fatalf("function parent metadata = %#v", fragment)
				}
			}
		})
	}
}

func TestShellTopLevelBodyIsIndependentFromFunctions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "script.zsh")
	content := `#!/bin/zsh
source_dir=$1
if [[ ! -d "$source_dir" ]]; then
  print -u2 -- "missing source"
  exit 1
fi

helper() {
  print -r -- "$1"
}

helper "$source_dir"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Zsh grammar not detected")
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "script.zsh", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 2 {
		t.Fatalf("fragments/warnings = %#v/%#v, want script and function", fragments, warnings)
	}
	topLevel := fragments[0]
	if topLevel.Location.FragmentKind != "script" || topLevel.Location.Name != "top-level" ||
		topLevel.NestedCount != 1 || topLevel.Features["node:function:nested"] != 1 {
		t.Fatalf("top-level fragment = %#v", topLevel)
	}
	if fragments[1].Location.FragmentKind != "function" || fragments[1].ParentID != topLevel.Fingerprint {
		t.Fatalf("function fragment = %#v", fragments[1])
	}

	changedPath := filepath.Join(t.TempDir(), "changed.zsh")
	changedContent := strings.Replace(content, `print -r -- "$1"`, `for value in "$@"; do print -r -- "$value"; done`, 1)
	if err := os.WriteFile(changedPath, []byte(changedContent), 0o600); err != nil {
		t.Fatalf("WriteFile changed: %v", err)
	}
	changed, changedWarnings := File(context.Background(), source.File{
		Path: changedPath, DisplayPath: "changed.zsh", Language: spec,
	}, 1)
	if len(changedWarnings) != 0 || len(changed) != 2 {
		t.Fatalf("changed fragments/warnings = %#v/%#v", changed, changedWarnings)
	}
	if changed[0].Fingerprint != topLevel.Fingerprint {
		t.Fatalf("function-body edit changed script fingerprint: %q != %q", changed[0].Fingerprint, topLevel.Fingerprint)
	}
	if changed[1].Fingerprint == fragments[1].Fingerprint {
		t.Fatal("function-body edit did not change function fingerprint")
	}
}

func TestJavaExtractsImplementedFunctionsAndNestedLambdas(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "Service.java")
	content := `
interface Worker {
  int requirement(int value);
}

record Profile(int value) {
  Profile {
    if (value < 0) throw new IllegalArgumentException();
  }
}

final class Service {
  private final int value;

  Service(int value) {
    this.value = value;
  }

  int process(int input) {
    java.util.function.Function<Integer, Integer> transform = item -> {
      return item + 1;
    };
    return transform.apply(input);
  }
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok || spec.ID != "java" {
		t.Fatalf("Java grammar = %q/%t", spec.ID, ok)
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "Service.java", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 4 {
		t.Fatalf("fragments/warnings = %#v/%#v, want four/none", fragments, warnings)
	}
	names := make(map[string]model.Fragment)
	for _, fragment := range fragments {
		names[fragment.Location.Name] = fragment
	}
	for _, name := range []string{"Profile", "Service", "process", "transform"} {
		if _, exists := names[name]; !exists {
			t.Errorf("missing Java fragment %q in %#v", name, names)
		}
	}
	if _, exists := names["requirement"]; exists {
		t.Fatal("bodyless Java interface method became a comparison fragment")
	}
	if nested := names["transform"]; nested.NestingDepth != 1 ||
		nested.Parent == nil || nested.Parent.Name != "process" {
		t.Fatalf("nested Java lambda = %#v", nested)
	}
}

func TestCSharpExtractsImplementedFunctionLikeBoundaries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "Service.cs")
	content := `
interface IWorker {
  int Requirement(int value);
}

sealed class Service {
  private int _value;

  public Service(int value) {
    _value = value;
  }

  ~Service() {
    Cleanup();
  }

  public int Process(int input) {
    System.Func<int, int> transform = item => item + 1;
    int Local(int item) { return item * 2; }
    return transform(Local(input));
  }

  public int Value {
    get { return _value; }
    set => _value = value;
  }

  public static Service operator +(Service left, Service right) =>
    new Service(left._value + right._value);

  private void Cleanup() {}
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok || spec.ID != "csharp" {
		t.Fatalf("C# grammar = %q/%t", spec.ID, ok)
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "Service.cs", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 9 {
		t.Fatalf("fragments/warnings = %#v/%#v, want nine/none", fragments, warnings)
	}
	names := make(map[string][]model.Fragment)
	for _, fragment := range fragments {
		names[fragment.Location.Name] = append(names[fragment.Location.Name], fragment)
	}
	for _, name := range []string{"Service", "Process", "transform", "Local", "get", "set", "Cleanup"} {
		if len(names[name]) == 0 {
			t.Errorf("missing C# fragment %q in %#v", name, names)
		}
	}
	if len(names["Requirement"]) != 0 {
		t.Fatal("bodyless C# interface method became a comparison fragment")
	}
	for _, name := range []string{"transform", "Local"} {
		nested := names[name][0]
		if nested.NestingDepth != 1 || nested.Parent == nil || nested.Parent.Name != "Process" {
			t.Fatalf("nested C# %s = %#v", name, nested)
		}
	}
}

func TestJavaAndCSharpMalformedFunctionsRemainVisible(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"Broken.java": `class Broken { int work( { return 1; } }`,
		"Broken.cs":   `class Broken { int Work( { return 1; } }`,
	}
	for name, content := range fixtures {
		name, content := name, content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, ok := language.Detect(path)
			if !ok {
				t.Fatalf("language not detected for %s", name)
			}
			_, warnings := File(context.Background(), source.File{
				Path: path, DisplayPath: name, Language: spec,
			}, 1)
			if len(warnings) != 1 || warnings[0].Kind != "parse" ||
				warnings[0].TotalDiagnostics == 0 {
				t.Fatalf("warnings = %#v, want visible parse diagnostics", warnings)
			}
		})
	}
}

func TestSwiftExtractsImplementedFunctionsAndClosures(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "Functions.swift")
	content := `protocol Worker {
  func requirement(value: Int) -> Int
}

func topLevel(value: Int) -> Int {
  return value + 1
}

final class Service {
  init(value: Int) {
    self.value = value
  }

  deinit {
    print("done")
  }

  func method(value: Int) -> Int {
    let transform = { (input: Int) -> Int in
      return input + 1
    }
    return transform(value)
  }

  var doubled: Int {
    value * 2
  }

  subscript(index: Int) -> Int {
    return index
  }

  let value: Int
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Swift grammar not detected")
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "Functions.swift", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 5 {
		t.Fatalf("fragments/warnings = %#v/%#v, want five/none", fragments, warnings)
	}
	names := make(map[string]bool)
	for _, fragment := range fragments {
		names[fragment.Location.Name] = true
		if fragment.Location.LanguageFamily != "swift" {
			t.Fatalf("Swift fragment = %#v", fragment.Location)
		}
	}
	for _, name := range []string{"topLevel", "init", "deinit", "method"} {
		if !names[name] {
			t.Errorf("missing Swift fragment name %q in %#v", name, names)
		}
	}
	if names["requirement"] {
		t.Fatal("bodyless protocol requirement became a comparison fragment")
	}
	if names["doubled"] || names["subscript"] {
		t.Fatal("computed property or subscript became a comparison fragment")
	}
	for _, fragment := range fragments {
		if fragment.Location.Name == "transform" && (fragment.NestingDepth != 1 ||
			fragment.Parent == nil || fragment.Parent.Name != "method") {
			t.Fatalf("nested Swift closure = %#v", fragment)
		}
	}
}

func TestSwiftMalformedFunctionProducesParseWarning(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "Broken.swift")
	if err := os.WriteFile(path, []byte("func broken(value: Int -> Int {\n  return value\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "Broken.swift", Language: spec,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "parse" ||
		warnings[0].Language != "swift" || warnings[0].TotalDiagnostics == 0 {
		t.Fatalf("fragments/warnings = %#v/%#v", fragments, warnings)
	}
}

func TestSwiftRepairsKnownValidSyntax(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"optional try await": `func refresh() async {
  if let response = try? await request {
    consume(response)
  }
}
`,
		"switch await": `func load() async {
  switch await task {
  case let .success(value):
    consume(value)
  case let .failure(error):
    consume(error)
  }
}
`,
		"empty tuple argument": `func clear() async {
  let result: Result<Void, Error> = .success(())
  consume(result)
}
`,
		"conditional cast with nil coalescing": `func version() -> String {
  let info = Bundle.main.infoDictionary ?? [:]
  let value = info["CFBundleVersion"] as? String ?? "?"
  return value
}
`,
		"labeled optional subscript cast chain": `func threadID(object: [String: Any]?, thread: [String: Any]?) -> String {
  let value = object?["threadId"] as? String ?? thread?["id"] as? String ?? "fallback"
  return value
}
`,
		"parenthesized dictionary cast chain": `func providers(root: [String: Any]) -> [[String: Any]] {
  let values = (root["all"] as? [[String: Any]] ?? root["providers"] as? [[String: Any]] ?? [])
  return values
}
`,
		"identifier collection cast chain": `func agents(payload: Any, root: [String: Any]) -> [[String: Any]] {
  let values = payload as? [[String: Any]] ?? root["agents"] as? [[String: Any]] ?? []
  return values
}
`,
		"dictionary cast chain": `func unwrap(payload: Any, root: [String: Any]) -> [String: Any] {
	let value = payload as? [String: Any] ?? root
	return root["data"] as? [String: Any] ?? value
}
`,
	}
	for name, content := range tests {
		name, content := name, content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "Valid.swift")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{
				Path: path, DisplayPath: "Valid.swift", Language: spec,
			}, 1)
			if len(warnings) != 0 || len(fragments) != 1 {
				t.Fatalf("fragments/warnings = %#v/%#v, want one/none", fragments, warnings)
			}
		})
	}
}

func TestSwiftRepairDoesNotHideMalformedNearbySyntax(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "Broken.swift")
	content := `func broken() async {
  if let response = try? await request {
    consume(response
  }
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "Broken.swift", Language: spec,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "parse" ||
		warnings[0].TotalDiagnostics == 0 {
		t.Fatalf("fragments/warnings = %#v/%#v, want visible malformed source", fragments, warnings)
	}
}

func TestPostgreSQLExtractsQueriesAndIgnoresDialectDDL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "migration.sql")
	content := `CREATE TABLE jobs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_state_check;

-- name: ArchiveJobs :exec
UPDATE jobs SET archived = TRUE WHERE created_at < now() RETURNING id;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_jobs_created
ON jobs (created_at DESC) INCLUDE (archived)
WHERE archived = FALSE;
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.DetectWithSQLDialect(path, language.SQLDialectPostgreSQL)
	if !ok {
		t.Fatal("PostgreSQL grammar not detected")
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "migration.sql", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 1 {
		t.Fatalf("fragments/warnings = %#v/%#v, want one/none", fragments, warnings)
	}
	location := fragments[0].Location
	if location.Name != "ArchiveJobs" || location.Language != "postgresql" ||
		location.LanguageFamily != "sql" || location.FragmentKind != "query" {
		t.Fatalf("PostgreSQL fragment = %#v", location)
	}
}

func TestPostgreSQLMalformedQueryProducesParseWarning(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.sql")
	if err := os.WriteFile(path, []byte("SELECT (1;\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.DetectWithSQLDialect(path, language.SQLDialectPostgreSQL)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "broken.sql", Language: spec,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "parse" {
		t.Fatalf("fragments/warnings = %#v/%#v, want none/one parse", fragments, warnings)
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

func TestFileExtractsOptInEmbeddedGoDatabaseSQL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "queries.go")
	content := `package sample
func load(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, ` + "`SELECT id FROM users WHERE tenant_id = $1 ORDER BY id`" + `, tenantID)
	_, _ = store.Exec(ctx, ` + "`UPDATE users SET active = TRUE WHERE tenant_id = $1`" + `, tenantID)
	log.Printf("SELECT id FROM ignored")
	runSQL("SELECT id FROM ignored_too")
	query := "SELECT id FROM variable_query"
	_, _ = rows, query
	return err
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := FileWithOptions(context.Background(), source.File{
		Path: path, DisplayPath: "queries.go", Language: spec,
	}, Options{
		MinTokens: 1, EmbeddedSQL: true, SQLDialect: language.SQLDialectPostgreSQL,
	})
	if len(warnings) != 0 || len(fragments) != 2 {
		t.Fatalf("fragments/warnings = %#v/%#v, want two/none", fragments, warnings)
	}
	fragment := fragments[0]
	if fragment.Location.Language != "postgresql" ||
		fragment.Location.ComparisonDomain != "sql-query" ||
		fragment.Location.FragmentKind != "query" ||
		fragment.Location.StartLine != 3 || fragments[1].Location.StartLine != 4 || fragment.Parent == nil ||
		fragment.Parent.Name != "load" || fragment.ParentID == "" {
		t.Fatalf("embedded SQL fragment = %#v", fragment)
	}
}

func TestFileReportsMalformedEmbeddedSQL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.go")
	content := "package sample\nfunc load(db *sql.DB) { db.Query(`SELECT (1;`) }\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := FileWithOptions(context.Background(), source.File{
		Path: path, DisplayPath: "broken.go", Language: spec,
	}, Options{MinTokens: 1, EmbeddedSQL: true, SQLDialect: language.SQLDialectPostgreSQL})
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "parse" ||
		!strings.Contains(warnings[0].Message, "host line 2") {
		t.Fatalf("fragments/warnings = %#v/%#v, want visible embedded parse warning", fragments, warnings)
	}
}

func TestFileTreatsOneEmbeddedStringAsOneQueryBatch(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "batch.go")
	content := "package sample\nfunc load(db *sql.DB) { db.Exec(`SELECT id FROM users; SELECT id FROM folders;`) }\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := FileWithOptions(context.Background(), source.File{
		Path: path, DisplayPath: "batch.go", Language: spec,
	}, Options{MinTokens: 1, EmbeddedSQL: true, SQLDialect: language.SQLDialectGeneric})
	if len(warnings) != 0 || len(fragments) != 1 ||
		fragments[0].Features["node:query:batch"] != 1 {
		t.Fatalf("fragments/warnings = %#v/%#v, want one query batch", fragments, warnings)
	}
}

func TestFileBoundsEmbeddedSQLPerFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "many.go")
	content := "package sample\nfunc load(db *sql.DB) {\n" +
		strings.Repeat("db.Query(`SELECT 1;`)\n", maxEmbeddedSQLCandidatesPerFile+1) + "}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := FileWithOptions(context.Background(), source.File{
		Path: path, DisplayPath: "many.go", Language: spec,
	}, Options{MinTokens: 1, EmbeddedSQL: true, SQLDialect: language.SQLDialectGeneric})
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "coverage" ||
		!strings.Contains(warnings[0].Message, "exceed per-file limit 1000") {
		t.Fatalf("fragments/warnings = %#v/%#v, want visible embedded-SQL cap", fragments, warnings)
	}
}

func TestFileExtractsBoundedStatementWindows(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "blocks.go")
	content := `package sample
func process(value string) error {
	clean := strings.TrimSpace(value)
	if clean == "" { return errors.New("empty") }
	result := strings.ToLower(clean)
	fmt.Println(result)
	return nil
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := FileWithOptions(context.Background(), source.File{
		Path: path, DisplayPath: "blocks.go", Language: spec,
	}, Options{MinTokens: 1, StatementBlocks: true, BlockStatements: 3, MaxBlocksPerFunc: 10})
	if len(warnings) != 0 || len(fragments) != 4 {
		t.Fatalf("fragments/warnings = %#v/%#v, want function plus three blocks", fragments, warnings)
	}
	for _, fragment := range fragments[1:] {
		if fragment.Location.FragmentKind != "block" || fragment.Parent == nil ||
			fragment.Parent.Name != "process" || fragment.ParentID != fragments[0].Fingerprint {
			t.Fatalf("block linkage = %#v", fragment)
		}
	}
}

func TestFileSkipsStatementWindowsOverConfiguredCap(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "blocks.go")
	content := `package sample
func process() {
	println(1)
	println(2)
	println(3)
	println(4)
	println(5)
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := FileWithOptions(context.Background(), source.File{
		Path: path, DisplayPath: "blocks.go", Language: spec,
	}, Options{MinTokens: 1, StatementBlocks: true, BlockStatements: 2, MaxBlocksPerFunc: 2})
	if len(fragments) != 1 || len(warnings) != 1 || warnings[0].Kind != "coverage" ||
		!strings.Contains(warnings[0].Message, "more than 2 windows would be emitted") {
		t.Fatalf("fragments/warnings = %#v/%#v, want function and visible cap warning", fragments, warnings)
	}
}

func TestStatementWindowsKeepNearbyStructuralNegative(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "negative.go")
	content := `package sample
func straight(value string) error {
	clean := strings.TrimSpace(value)
	fmt.Println(clean)
	return nil
}
func branching(values []string) error {
	for _, value := range values { fmt.Println(value) }
	if len(values) == 0 { return errors.New("empty") }
	return nil
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := FileWithOptions(context.Background(), source.File{
		Path: path, DisplayPath: "negative.go", Language: spec,
	}, Options{MinTokens: 1, StatementBlocks: true, BlockStatements: 3, MaxBlocksPerFunc: 10})
	if len(warnings) != 0 || len(fragments) != 4 {
		t.Fatalf("fragments/warnings = %#v/%#v", fragments, warnings)
	}
	if fragments[1].Location.FragmentKind != "block" ||
		fragments[3].Location.FragmentKind != "block" ||
		fragments[1].Fingerprint == fragments[3].Fingerprint {
		t.Fatalf("nearby negative blocks = %#v / %#v", fragments[1], fragments[3])
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

func TestPHPAndHackExtractImplementedFunctions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		content  string
		language string
	}{
		"functions.php": {
			language: "php",
			content: `<?php
interface Worker {
  public function requirement(int $value): int;
}

function topLevel(int $value): int {
  return $value + 1;
}

final class Service {
  public function method(int $value): int {
    $transform = fn(int $input): int => $input + 1;
    return $transform($value);
  }
}
`,
		},
		"functions.hack": {
			language: "hack",
			content: `function topLevel(int $value): int {
  return $value + 1;
}

interface Worker {
  public function requirement(int $value): int;
}

final class Service {
  public function method(int $value): int {
    $transform = (int $input): int ==> $input + 1;
    return $transform($value);
  }
}
`,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, ok := language.Detect(path)
			if !ok || spec.ID != test.language {
				t.Fatalf("Detect(%s) = %q/%t", name, spec.ID, ok)
			}
			fragments, warnings := File(context.Background(), source.File{
				Path: path, DisplayPath: name, Language: spec,
			}, 1)
			if len(warnings) != 0 || len(fragments) != 3 {
				t.Fatalf("fragments/warnings = %#v/%#v, want three/none", fragments, warnings)
			}
			names := make(map[string]bool)
			for _, fragment := range fragments {
				names[fragment.Location.Name] = true
				if fragment.Location.LanguageFamily != "php-hack" {
					t.Fatalf("fragment family = %q, want php-hack", fragment.Location.LanguageFamily)
				}
			}
			if !names["topLevel"] || !names["method"] || names["requirement"] {
				t.Fatalf("fragment names = %#v", names)
			}
		})
	}
}

func TestLegacyHackShebangPreservesLocations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "legacy.php")
	content := "#!/usr/bin/env hhvm\n<?hh // strict\nfunction run(int $value): int {\n  return $value + 1;\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.DetectWithSourcePrefix(path, language.SQLDialectGeneric, []byte(content))
	if !ok || spec.ID != "hack" {
		t.Fatalf("legacy Hack detection = %q/%t", spec.ID, ok)
	}
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "legacy.php", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 1 {
		t.Fatalf("fragments/warnings = %#v/%#v", fragments, warnings)
	}
	if fragments[0].Location.StartLine != 3 || fragments[0].Location.Name != "run" {
		t.Fatalf("legacy fragment location = %#v", fragments[0].Location)
	}
}

func TestHackMalformedFunctionProducesParseWarning(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.hack")
	if err := os.WriteFile(path, []byte("function broken(int $value: int {\n  return $value;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{
		Path: path, DisplayPath: "broken.hack", Language: spec,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 || warnings[0].Kind != "parse" ||
		warnings[0].Language != "hack" || warnings[0].TotalDiagnostics == 0 {
		t.Fatalf("fragments/warnings = %#v/%#v", fragments, warnings)
	}
}
