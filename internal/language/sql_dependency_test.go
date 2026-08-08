package language

import (
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_sql "github.com/wippyai/tree-sitter-sql/bindings/go"
)

func TestSQLDependencyParsesRepresentativeQueries(t *testing.T) {
	t.Parallel()

	language := tree_sitter.NewLanguage(tree_sitter_sql.Language())
	if got, want := language.AbiVersion(), uint32(14); got != want {
		t.Fatalf("SQL ABI = %d, want %d", got, want)
	}
	if language.AbiVersion() < tree_sitter.MIN_COMPATIBLE_LANGUAGE_VERSION ||
		language.AbiVersion() > tree_sitter.LANGUAGE_VERSION {
		t.Fatalf(
			"SQL ABI = %d, supported range is %d-%d",
			language.AbiVersion(),
			tree_sitter.MIN_COMPATIBLE_LANGUAGE_VERSION,
			tree_sitter.LANGUAGE_VERSION,
		)
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		t.Fatalf("SetLanguage(SQL): %v", err)
	}
	source := []byte(`-- name: ListFolders :many
WITH visible AS (
  SELECT id, tenant_id FROM folders WHERE tenant_id = ?
)
SELECT id FROM visible ORDER BY id;

UPDATE folders SET name = $1 WHERE id = $2 AND tenant_id = $3;
`)
	tree := parser.Parse(source, nil)
	if tree == nil {
		t.Fatal("SQL parser returned no tree")
	}
	defer tree.Close()
	if tree.RootNode().HasError() {
		t.Fatalf("representative SQL has parse errors: %s", tree.RootNode().ToSexp())
	}
}
