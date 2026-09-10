package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

func TestAmbientImportTypeCompatibility(t *testing.T) {
	t.Parallel()
	for _, extension := range []string{".ts", ".tsx"} {
		for name, declaration := range map[string]string{
			"namespace": `declare namespace Example { interface Env { ITEMS: import("example:test").Record[]; } }`,
			"module":    `declare module "example:test" { interface Env { ITEMS: import('example:types').Record[]; } }`,
		} {
			t.Run(name+extension, func(t *testing.T) {
				content := declaration + "\nfunction keep(value: string) { return value.trim(); }\n"
				path := filepath.Join(t.TempDir(), "input"+extension)
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				spec, _ := language.Detect(path)
				fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input" + extension, Language: spec}, 1)
				if len(warnings) != 0 {
					t.Fatalf("warnings = %+v", warnings)
				}
				if len(fragments) != 1 || fragments[0].Location.Name != "keep" || fragments[0].Location.StartLine != 2 {
					t.Fatalf("fragments = %+v", fragments)
				}
			})
		}
	}
}

func TestSQLiteStrictDDLRemainsOutsideQueryCoverage(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "schema.sql")
	if err := os.WriteFile(path, []byte(`CREATE TABLE records (id INTEGER PRIMARY KEY, label TEXT NOT NULL) STRICT;`), 0600); err != nil {
		t.Fatal(err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings, coverage := FileWithCoverage(context.Background(), source.File{Path: path, DisplayPath: "schema.sql", Language: spec}, Options{MinTokens: 1})
	if len(fragments) != 0 || coverage.CandidateFragments != 0 {
		t.Fatalf("DDL unexpectedly compared: %+v / %+v", fragments, coverage)
	}
	if len(warnings) != 0 {
		t.Fatalf("valid DDL must parse without becoming a query: %+v", warnings)
	}
}

func TestAmbientImportRepairDoesNotScoreInvalidFunction(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "input.ts")
	content := `declare namespace Example { interface Env { ITEMS: import("example:test").Record[]; } }
function broken(value: string) { return value + ; }
function valid(value: string) { return value.trim(); }
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.ts", Language: spec}, 1)
	if len(fragments) != 1 || fragments[0].Location.Name != "valid" {
		t.Fatalf("invalid function must remain excluded: %+v", fragments)
	}
	if len(warnings) != 1 || warnings[0].Kind != "parse" || warnings[0].SkippedFragments != 1 {
		t.Fatalf("invalid function diagnostic lost: %+v", warnings)
	}
}

func TestSwiftMethodResultCastCompatibility(t *testing.T) {
	t.Parallel()
	for name, expression := range map[string]string{
		"literal_argument":    `defaults.object(forKey: "opacity") as? Double ?? 0.94`,
		"identifier_argument": `defaults.object(forKey: key) as? Bool ?? false`,
		"property_receiver":   `settings.defaults.object(forKey: "opacity") as? Double ?? 0.94`,
	} {
		t.Run(name, func(t *testing.T) {
			content := "func load() {\n let value = " + expression + "\n consume(value)\n}\n"
			path := filepath.Join(t.TempDir(), "input.swift")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.swift", Language: spec}, 1)
			if len(warnings) != 0 || len(fragments) != 1 || fragments[0].Location.EndLine != 4 {
				t.Fatalf("fragments/warnings = %+v / %+v", fragments, warnings)
			}
		})
	}
}

func TestSwiftMethodResultCastKeepsMalformedSyntaxVisible(t *testing.T) {
	t.Parallel()
	for name, expression := range map[string]string{
		"missing_operand":     `defaults.object(forKey: "x" +) as? Double ?? 0.94`,
		"missing_parenthesis": `defaults.object(forKey: "opacity" as? Double ?? 0.94`,
		"missing_type":        `defaults.object(forKey: "opacity") as? ?? 0.94`,
		"broken_neighbor":     `defaults.object(forKey: "opacity") as? Double ?? 0.94; broken(, )`,
	} {
		t.Run(name, func(t *testing.T) {
			content := "func load() {\n let value = " + expression + "\n consume(value)\n}\n"
			path := filepath.Join(t.TempDir(), "input.swift")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.swift", Language: spec}, 1)
			if len(warnings) == 0 || len(fragments) != 0 {
				t.Fatalf("fragments/warnings = %+v / %+v", fragments, warnings)
			}
		})
	}
}

func TestSQLiteConsumerGrammarForms(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"pragma_assignment":  `PRAGMA foreign_keys = ON;`,
		"pragma_argument":    `PRAGMA table_info(records);`,
		"strict_table":       `CREATE TABLE records (id INTEGER PRIMARY KEY, label TEXT NOT NULL) STRICT;`,
		"between_constraint": `CREATE TABLE records (label TEXT CHECK (length(label) BETWEEN 1 AND 128));`,
		"glob_constraint":    `CREATE TABLE records (digest TEXT CHECK (digest NOT GLOB '*[^0-9a-f]*'));`,
		"trigger":            `CREATE TRIGGER records_guard BEFORE INSERT ON records WHEN EXISTS (SELECT 1 FROM removed WHERE id = NEW.id) BEGIN SELECT RAISE(ABORT, 'removed'); END;`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.sql")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.sql", Language: spec}, 1)
			if len(fragments) != 0 {
				t.Fatalf("DDL/PRAGMA became comparable queries: %+v", fragments)
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %+v", warnings)
			}
		})
	}
}

func TestSQLiteConsumerGrammarRejectsMalformedForms(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"pragma_missing_value":  `PRAGMA foreign_keys = ;`,
		"pragma_unclosed":       `PRAGMA table_info(records;`,
		"strict_invalid_table":  `CREATE TABLE records (id INTEGER PRIMARY KEY, ) STRICT;`,
		"between_missing_bound": `CREATE TABLE records (label TEXT CHECK (length(label) BETWEEN 1 AND));`,
		"glob_missing_pattern":  `CREATE TABLE records (digest TEXT CHECK (digest NOT GLOB));`,
		"trigger_missing_end":   `CREATE TRIGGER records_guard BEFORE INSERT ON records BEGIN SELECT RAISE(ABORT, 'removed');`,
		"trigger_broken_select": `CREATE TRIGGER records_guard BEFORE INSERT ON records BEGIN SELECT * FROM; END;`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.sql")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			_, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.sql", Language: spec}, 1)
			if len(warnings) == 0 {
				t.Fatalf("malformed SQL has no warning")
			}
		})
	}
}
