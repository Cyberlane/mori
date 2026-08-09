package normalize_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/parser"
	"github.com/Cyberlane/mori/internal/similarity"
	"github.com/Cyberlane/mori/internal/source"
)

func TestRenamesStaySimilarAndDifferentLogicDoesNot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fixtures.js")
	content := `
function first(address) {
  const clean = address.trim();
  return clean.includes("@") && clean.includes(".");
}

function renamed(candidate) {
  const normalized = candidate.trim();
  return normalized.includes("@") && normalized.includes(".");
}

function total(values) {
  let result = 0;
  for (const value of values) {
    result += value;
  }
  return result;
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("JavaScript grammar not detected")
	}
	fragments, warnings := parser.File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "fixtures.js",
		Language:    spec,
	}, 1)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(fragments) != 3 {
		t.Fatalf("fragments = %d, want 3", len(fragments))
	}

	byName := make(map[string]model.Fragment, len(fragments))
	for _, fragment := range fragments {
		byName[fragment.Location.Name] = fragment
	}
	renamedScore, _, _ := similarity.WeightedJaccard(
		byName["first"].Features,
		byName["renamed"].Features,
	)
	if renamedScore < 0.95 {
		t.Fatalf("renamed score = %.3f, want at least 0.95", renamedScore)
	}

	unrelatedScore, _, _ := similarity.WeightedJaccard(
		byName["first"].Features,
		byName["total"].Features,
	)
	if unrelatedScore >= 0.60 {
		t.Fatalf("unrelated score = %.3f, want below 0.60", unrelatedScore)
	}
}

func TestLiteralDigestsExposeDriftWithoutChangingFingerprint(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "literals.js")
	content := `function first() { return "jpeg"; }
function second() { return "avif"; }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, _ := language.Detect(path)
	fragments, warnings := parser.File(context.Background(), source.File{
		Path: path, DisplayPath: "literals.js", Language: spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 2 {
		t.Fatalf("fragments/warnings = %#v/%#v", fragments, warnings)
	}
	if fragments[0].Fingerprint != fragments[1].Fingerprint {
		t.Fatalf("literal values changed fingerprint: %s != %s", fragments[0].Fingerprint, fragments[1].Fingerprint)
	}
	if len(fragments[0].LiteralDigests) != 1 || len(fragments[1].LiteralDigests) != 1 ||
		reflect.DeepEqual(fragments[0].LiteralDigests, fragments[1].LiteralDigests) {
		t.Fatalf("literal digests = %#v/%#v, want one differing position", fragments[0].LiteralDigests, fragments[1].LiteralDigests)
	}
}

func TestGoStatementListWrapperIsTransparent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "statements.go")
	content := `package sample

func normalize(value string) string {
	cleaned := value
	return cleaned
}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Go grammar not detected")
	}
	fragments, warnings := parser.File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "statements.go",
		Language:    spec,
	}, 1)
	if len(warnings) != 0 || len(fragments) != 1 {
		t.Fatalf("warnings/fragments = %#v/%d, want none/1", warnings, len(fragments))
	}

	features := fragments[0].Features
	for feature := range features {
		if strings.Contains(feature, "statement_list") {
			t.Fatalf("grammar-only statement list leaked into feature %q", feature)
		}
	}
	if got := features["edge:block>binding"]; got != 1 {
		t.Errorf("edge:block>binding count = %d, want 1", got)
	}
	if got := features["edge:block>flow:return"]; got != 1 {
		t.Errorf("edge:block>flow:return count = %d, want 1", got)
	}
}

func TestShellGrammarAliasesPreservePositiveAndNegativeSeparation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtures := map[string]string{
		"positive.sh": `validate_email() {
  value=$1
  if [ -z "$value" ]; then return 1; fi
  case "$value" in *@*.*) return 0 ;; *) return 1 ;; esac
}
`,
		"renamed.zsh": `check_address() {
  candidate=$1
  if [ -z "$candidate" ]; then return 1; fi
  case "$candidate" in *@*.*) return 0 ;; *) return 1 ;; esac
}
`,
		"nearby.zsh": `report_items() {
  for item in "$@"; do print -r -- "item: $item"; done
}
`,
	}
	fragments := make(map[string]model.Fragment, len(fixtures))
	for name, content := range fixtures {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
		spec, ok := language.Detect(path)
		if !ok {
			t.Fatalf("Detect(%s) returned unsupported", name)
		}
		parsed, warnings := parser.File(context.Background(), source.File{
			Path: path, DisplayPath: name, Language: spec,
		}, 1)
		if len(warnings) != 0 || len(parsed) != 2 {
			t.Fatalf("%s warnings/fragments = %#v/%d", name, warnings, len(parsed))
		}
		fragments[name] = parsed[1]
	}

	positive, _, _ := similarity.WeightedJaccard(
		fragments["positive.sh"].Features,
		fragments["renamed.zsh"].Features,
	)
	nearby, _, _ := similarity.WeightedJaccard(
		fragments["positive.sh"].Features,
		fragments["nearby.zsh"].Features,
	)
	if positive < 0.90 {
		t.Fatalf("shell positive score = %.3f, want at least 0.90", positive)
	}
	if nearby >= 0.60 {
		t.Fatalf("shell nearby-negative score = %.3f, want below 0.60", nearby)
	}
	for feature := range fragments["renamed.zsh"].Features {
		if strings.Contains(feature, "variable_ref") || strings.Contains(feature, "variable_name") ||
			strings.Contains(feature, "glob_pattern") || strings.Contains(feature, "extglob_pattern") {
			t.Fatalf("Zsh grammar vocabulary leaked into feature %q", feature)
		}
	}
}

func TestShellTopLevelBodiesPreservePositiveAndNegativeSeparation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtures := map[string]string{
		"positive.sh": `source_dir=$1
if [ ! -d "$source_dir" ]; then
  printf '%s\n' "missing source" >&2
  exit 1
fi
selected=$(find "$source_dir" -type f | head -n 1)
if [ -z "$selected" ]; then exit 1; fi
printf '%s\n' "$selected"
`,
		"renamed.zsh": `input_root=$1
if [ ! -d "$input_root" ]; then
  printf '%s\n' "invalid folder" >&2
  exit 1
fi
choice=$(find "$input_root" -type f | head -n 1)
if [ -z "$choice" ]; then exit 1; fi
printf '%s\n' "$choice"
`,
		"nearby.zsh": `for item in "$@"; do
  printf '%s\n' "item: $item"
done
`,
	}
	fragments := make(map[string]model.Fragment, len(fixtures))
	for name, content := range fixtures {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
		spec, ok := language.Detect(path)
		if !ok {
			t.Fatalf("Detect(%s) returned unsupported", name)
		}
		parsed, warnings := parser.File(context.Background(), source.File{
			Path: path, DisplayPath: name, Language: spec,
		}, 1)
		if len(warnings) != 0 || len(parsed) != 1 || parsed[0].Location.FragmentKind != "script" {
			t.Fatalf("%s warnings/fragments = %#v/%#v", name, warnings, parsed)
		}
		fragments[name] = parsed[0]
	}

	positive, _, _ := similarity.WeightedJaccard(
		fragments["positive.sh"].Features,
		fragments["renamed.zsh"].Features,
	)
	nearby, _, _ := similarity.WeightedJaccard(
		fragments["positive.sh"].Features,
		fragments["nearby.zsh"].Features,
	)
	if positive < 0.90 {
		t.Fatalf("shell top-level positive score = %.3f, want at least 0.90", positive)
	}
	if nearby >= 0.60 {
		t.Fatalf("shell top-level nearby-negative score = %.3f, want below 0.60", nearby)
	}
}

func TestSwiftMappingsPreservePositiveAndNegativeSeparation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtures := map[string]string{
		"positive.go": `package sample
func clampNumber(value int, lower int, upper int) int {
  if value < lower { return lower }
  if value > upper { return upper }
  return value
}
`,
		"renamed.swift": `func clampValue(_ value: Int, minimum: Int, maximum: Int) -> Int {
  if value < minimum { return minimum }
  if value > maximum { return maximum }
  return value
}
`,
		"nearby.swift": `func sumValues(_ values: [Int]) -> Int {
  var total = 0
  for value in values { total += value }
  return total
}
`,
	}
	fragments := make(map[string]model.Fragment, len(fixtures))
	for name, content := range fixtures {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
		spec, ok := language.Detect(path)
		if !ok {
			t.Fatalf("Detect(%s) returned unsupported", name)
		}
		parsed, warnings := parser.File(context.Background(), source.File{
			Path: path, DisplayPath: name, Language: spec,
		}, 1)
		if len(warnings) != 0 || len(parsed) != 1 {
			t.Fatalf("%s warnings/fragments = %#v/%d", name, warnings, len(parsed))
		}
		fragments[name] = parsed[0]
	}

	positive, _, _ := similarity.WeightedJaccard(
		fragments["positive.go"].Features,
		fragments["renamed.swift"].Features,
	)
	nearby, _, _ := similarity.WeightedJaccard(
		fragments["positive.go"].Features,
		fragments["nearby.swift"].Features,
	)
	if positive < 0.80 {
		t.Fatalf("Swift positive score = %.3f, want at least 0.80", positive)
	}
	if nearby >= 0.60 {
		t.Fatalf("Swift nearby-negative score = %.3f, want below 0.60", nearby)
	}
	for feature := range fragments["renamed.swift"].Features {
		if strings.Contains(feature, "control_transfer") || strings.Contains(feature, "user_type") ||
			strings.Contains(feature, "syntax:statements") {
			t.Fatalf("Swift grammar vocabulary leaked into feature %q", feature)
		}
	}
}

func TestPostgreSQLMappingsPreservePositiveAndNegativeSeparation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtures := map[string]string{
		"users.sql": `SELECT id, email
FROM users
WHERE status = $1
ORDER BY created_at DESC;
`,
		"accounts.sql": `SELECT account_id, contact_email
FROM accounts
WHERE state = $1
ORDER BY opened_at DESC;
`,
		"archive.sql": `UPDATE sessions
SET archived = TRUE
WHERE expires_at < NOW()
RETURNING id;
`,
	}
	fragments := make(map[string]model.Fragment, len(fixtures))
	for name, content := range fixtures {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
		spec, ok := language.DetectWithSQLDialect(path, language.SQLDialectPostgreSQL)
		if !ok {
			t.Fatalf("DetectWithSQLDialect(%s) returned unsupported", name)
		}
		parsed, warnings := parser.File(context.Background(), source.File{
			Path: path, DisplayPath: name, Language: spec,
		}, 1)
		if len(warnings) != 0 || len(parsed) != 1 {
			t.Fatalf("%s warnings/fragments = %#v/%d", name, warnings, len(parsed))
		}
		fragments[name] = parsed[0]
	}

	positive, _, _ := similarity.WeightedJaccard(
		fragments["users.sql"].Features,
		fragments["accounts.sql"].Features,
	)
	nearby, _, _ := similarity.WeightedJaccard(
		fragments["users.sql"].Features,
		fragments["archive.sql"].Features,
	)
	if positive < 0.90 {
		t.Fatalf("PostgreSQL positive score = %.3f, want at least 0.90", positive)
	}
	if nearby >= 0.65 {
		t.Fatalf("PostgreSQL nearby-negative score = %.3f, want below 0.65", nearby)
	}
	for feature := range fragments["users.sql"].Features {
		if strings.Contains(feature, "syntax:kw_") || strings.Contains(feature, "syntax:a_expr") ||
			strings.Contains(feature, "syntax:opt_") || strings.Contains(feature, "syntax:ColId") {
			t.Fatalf("PostgreSQL grammar vocabulary leaked into feature %q", feature)
		}
	}
}

func TestSemanticOperationFamiliesAndNearbyNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		positive string
		nearby   string
		feature  string
	}{
		{name: "membership", positive: "value.includes('x')", nearby: "value.includesOther('x')", feature: "membership"},
		{name: "pattern", positive: "value.matches('x')", nearby: "value.matchesOther('x')", feature: "pattern-match"},
		{name: "length", positive: "len(value)", nearby: "lenOther(value)", feature: "length"},
		{name: "lowercase", positive: "value.toLowerCase()", nearby: "value.toLowerCaseOther()", feature: "lowercase"},
		{name: "uppercase", positive: "value.toUpperCase()", nearby: "value.toUpperCaseOther()", feature: "uppercase"},
		{name: "trim", positive: "value.trim()", nearby: "value.trimOther()", feature: "trim"},
		{name: "filter", positive: "value.filter(predicate)", nearby: "value.filterOther(predicate)", feature: "filter"},
		{name: "map", positive: "value.map(transform)", nearby: "value.mapOther(transform)", feature: "map"},
		{name: "reduce", positive: "value.reduce(reducer)", nearby: "value.reduceOther(reducer)", feature: "reduce"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), test.name+".js")
			content := "function positive(value) { return " + test.positive + "; }\n" +
				"function nearby(value) { return " + test.nearby + "; }\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, ok := language.Detect(path)
			if !ok {
				t.Fatal("JavaScript grammar not detected")
			}
			fragments, warnings := parser.File(context.Background(), source.File{
				Path:        path,
				DisplayPath: filepath.Base(path),
				Language:    spec,
			}, 1)
			if len(warnings) != 0 || len(fragments) != 2 {
				t.Fatalf(
					"warnings/fragments = %#v/%d, want none/2",
					warnings,
					len(fragments),
				)
			}

			byName := make(map[string]model.Fragment, len(fragments))
			for _, fragment := range fragments {
				byName[fragment.Location.Name] = fragment
			}
			feature := "semantic:" + test.feature
			if got := byName["positive"].Features[feature]; got != 2 {
				t.Errorf("positive %s count = %d, want 2", feature, got)
			}
			if got := byName["nearby"].Features[feature]; got != 0 {
				t.Errorf("nearby %s count = %d, want 0", feature, got)
			}
		})
	}
}

func TestSQLQueryPositivesOutrankNearbyNegatives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		positive string
		renamed  string
		nearby   string
		feature  string
	}{
		{
			name: "select",
			positive: `SELECT u.id, COUNT(o.id) FROM users u LEFT JOIN orders o ON o.user_id = u.id
WHERE u.tenant_id = $1 GROUP BY u.id HAVING COUNT(o.id) > 0 ORDER BY u.id LIMIT 10`,
			renamed: `SELECT a.key, COUNT(b.key) FROM accounts a LEFT JOIN purchases b ON b.account_key = a.key
WHERE a.organization_key = $9 GROUP BY a.key HAVING COUNT(b.key) > 7 ORDER BY a.key LIMIT 25`,
			nearby: `SELECT u.id, COUNT(o.id) FROM users u INNER JOIN orders o ON o.user_id = u.id
GROUP BY u.id HAVING COUNT(o.id) > 0 ORDER BY u.id LIMIT 10`,
			feature: "node:query:select",
		},
		{
			name:     "cte",
			positive: `WITH visible AS (SELECT id FROM folders WHERE tenant_id = $1) SELECT id FROM visible`,
			renamed:  `WITH allowed AS (SELECT key FROM directories WHERE organization_id = $7) SELECT key FROM allowed`,
			nearby:   `SELECT id FROM folders WHERE tenant_id = $1`,
			feature:  "node:query:cte",
		},
		{
			name:     "insert",
			positive: `INSERT INTO users (tenant_id, email) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING id`,
			renamed:  `INSERT INTO members (organization_id, address) VALUES ($8, $9) ON CONFLICT DO NOTHING RETURNING key`,
			nearby:   `INSERT INTO users (tenant_id, email) VALUES ($1, $2) RETURNING id`,
			feature:  "node:query:insert",
		},
		{
			name:     "update",
			positive: `UPDATE users SET email = $1 WHERE id = $2 AND tenant_id = $3 RETURNING id`,
			renamed:  `UPDATE members SET address = $9 WHERE key = $8 AND organization_id = $7 RETURNING key`,
			nearby:   `UPDATE users SET email = $1 WHERE id = $2 RETURNING id`,
			feature:  "node:query:update",
		},
		{
			name:     "delete",
			positive: `DELETE FROM users WHERE id = $1 AND tenant_id = $2 RETURNING id`,
			renamed:  `DELETE FROM members WHERE key = $8 AND organization_id = $9 RETURNING key`,
			nearby:   `UPDATE users SET deleted_at = $3 WHERE id = $1 AND tenant_id = $2 RETURNING id`,
			feature:  "node:query:delete",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fragments := parseSQL(t, test.positive+";\n"+test.renamed+";\n"+test.nearby+";\n")
			if len(fragments) != 3 {
				t.Fatalf("fragments = %d, want 3", len(fragments))
			}
			if fragments[0].Features[test.feature] == 0 {
				t.Fatalf("positive query missing %s: %#v", test.feature, fragments[0].Features)
			}
			positiveScore, _, _ := similarity.WeightedJaccard(fragments[0].Features, fragments[1].Features)
			nearbyScore, _, _ := similarity.WeightedJaccard(fragments[0].Features, fragments[2].Features)
			if positiveScore <= nearbyScore {
				t.Fatalf("positive %.3f did not outrank nearby negative %.3f", positiveScore, nearbyScore)
			}
			if fragments[0].Fingerprint != fragments[1].Fingerprint {
				t.Fatalf("identifier/literal renaming changed fingerprint: %s != %s", fragments[0].Fingerprint, fragments[1].Fingerprint)
			}
		})
	}
}

func TestSQLSemanticHintsHaveNearbyNegatives(t *testing.T) {
	t.Parallel()

	fragments := parseSQL(t, `SELECT COUNT(id), sqlc.arg(tenant_id) FROM users WHERE tenant_id IN ($1, $2);
	SELECT LOWER(id), custom.argOther(tenant_id) FROM users WHERE tenant_id = $1;
`)
	if len(fragments) != 2 {
		t.Fatalf("fragments = %d, want 2", len(fragments))
	}
	for feature, want := range map[string]int{
		"semantic:aggregate":  2,
		"semantic:membership": 2,
		"semantic:parameter":  2,
	} {
		if got := fragments[0].Features[feature]; got != want {
			t.Errorf("positive %s = %d, want %d", feature, got, want)
		}
		if got := fragments[1].Features[feature]; got != 0 {
			t.Errorf("nearby %s = %d, want 0", feature, got)
		}
	}
}

func TestSQLLimitParametersOutrankLiteralPagination(t *testing.T) {
	t.Parallel()

	fragments := parseSQL(t, `SELECT id FROM items LIMIT ? OFFSET ?;
SELECT item_id FROM records LIMIT sqlc.arg(limit) OFFSET sqlc.narg(offset);
SELECT item_id FROM records LIMIT 20 OFFSET 40;
`)
	if len(fragments) != 3 {
		t.Fatalf("fragments = %d, want 3", len(fragments))
	}
	parameterScore, _, _ := similarity.WeightedJaccard(fragments[0].Features, fragments[1].Features)
	literalScore, _, _ := similarity.WeightedJaccard(fragments[0].Features, fragments[2].Features)
	if parameterScore <= literalScore {
		t.Fatalf("parameter score %.3f did not outrank literal pagination %.3f", parameterScore, literalScore)
	}
	if fragments[0].Features["node:parameter"] == 0 || fragments[1].Features["node:parameter"] == 0 ||
		fragments[2].Features["node:parameter"] != 0 {
		t.Fatalf("parameter features = %#v / %#v / %#v", fragments[0].Features, fragments[1].Features, fragments[2].Features)
	}
}

func TestSQLKeywordsAndValuesDoNotLeakIntoFeatures(t *testing.T) {
	t.Parallel()

	fragments := parseSQL(t, `SELECT customer_name FROM customers WHERE tenant_id = $17 AND active = true LIMIT 42;`)
	if len(fragments) != 1 {
		t.Fatalf("fragments = %d, want 1", len(fragments))
	}
	for feature := range fragments[0].Features {
		if strings.Contains(feature, "customer") || strings.Contains(feature, "tenant_id") ||
			strings.Contains(feature, "$17") || strings.Contains(feature, "42") ||
			strings.Contains(feature, "keyword_") {
			t.Fatalf("source spelling leaked into feature %q", feature)
		}
	}
	for _, feature := range []string{
		"node:literal:boolean", "node:literal:number", "node:parameter", "semantic:membership",
	} {
		if feature == "semantic:membership" {
			continue
		}
		if fragments[0].Features[feature] == 0 {
			t.Errorf("normalized query missing %s: %#v", feature, fragments[0].Features)
		}
	}
}

func parseSQL(t *testing.T, content string) []model.Fragment {
	t.Helper()
	path := filepath.Join(t.TempDir(), "queries.sql")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("SQL grammar not detected")
	}
	fragments, warnings := parser.File(context.Background(), source.File{
		Path: path, DisplayPath: "queries.sql", Language: spec,
	}, 1)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	return fragments
}
