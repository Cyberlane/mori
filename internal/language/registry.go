// Package language owns supported Tree-sitter grammars and fragment boundaries.
package language

import (
	"path/filepath"
	"sort"
	"strings"

	tree_sitter_postgresql "github.com/Cyberlane/mori/internal/grammar/postgresql"
	tree_sitter_swift "github.com/Cyberlane/mori/internal/grammar/swift"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
	tree_sitter_zsh "github.com/tree-sitter/tree-sitter-zsh/bindings/go"
	tree_sitter_sql "github.com/wippyai/tree-sitter-sql/bindings/go"
)

// Spec describes a parser grammar and the nodes that form comparison units.
type Spec struct {
	ID                      string
	Family                  string
	ComparisonDomain        string
	FragmentKind            string
	DisplayName             string
	Extensions              []string
	Shebangs                []string
	newLanguage             func() *tree_sitter.Language
	fragmentKinds           map[string]struct{}
	topLevelKinds           map[string]struct{}
	topLevelFragmentKind    string
	acceptBoundary          func(*tree_sitter.Node) bool
	excludeNestedBoundaries bool
}

// TopLevelFragmentKind returns the separate comparison kind for a recognized
// whole-file boundary, or an empty string when the grammar has none.
func (s Spec) TopLevelFragmentKind(node *tree_sitter.Node) string {
	if node == nil {
		return ""
	}
	if _, ok := s.topLevelKinds[node.Kind()]; !ok {
		return ""
	}
	return s.topLevelFragmentKind
}

// FragmentKinds returns the independently compared fragment kinds emitted by
// this grammar.
func (s Spec) FragmentKinds() []string {
	kinds := []string{s.FragmentKind}
	if s.topLevelFragmentKind != "" && s.topLevelFragmentKind != s.FragmentKind {
		kinds = append(kinds, s.topLevelFragmentKind)
	}
	sort.Strings(kinds)
	return kinds
}

// NewLanguage returns a Tree-sitter language wrapper for this grammar.
func (s Spec) NewLanguage() *tree_sitter.Language {
	return s.newLanguage()
}

// IsFragmentBoundary reports whether kind can form a comparison boundary.
func (s Spec) IsFragmentBoundary(kind string) bool {
	_, ok := s.fragmentKinds[kind]
	return ok
}

// AcceptsFragmentBoundary applies any grammar-specific top-level boundary
// qualification after the node kind is recognized.
func (s Spec) AcceptsFragmentBoundary(node *tree_sitter.Node) bool {
	if node == nil || !s.IsFragmentBoundary(node.Kind()) {
		return false
	}
	return s.acceptBoundary == nil || s.acceptBoundary(node)
}

// ExcludesNestedBoundaries reports whether nested boundary bodies are
// independent fragments and therefore excluded from their parent profile.
func (s Spec) ExcludesNestedBoundaries() bool {
	return s.excludeNestedBoundaries
}

var javascriptFunctions = kinds(
	"arrow_function",
	"function_declaration",
	"function_expression",
	"generator_function",
	"generator_function_declaration",
	"method_definition",
)

const (
	SQLDialectGeneric    = "generic"
	SQLDialectPostgreSQL = "postgresql"
)

var specs = []Spec{
	{
		ID:               "bash",
		Family:           "shell",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "Bash / POSIX shell",
		Extensions:       []string{".bash", ".sh"},
		Shebangs:         []string{"bash", "dash", "sh"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_bash.Language())
		},
		fragmentKinds:           kinds("function_definition"),
		topLevelKinds:           kinds("program"),
		topLevelFragmentKind:    "script",
		excludeNestedBoundaries: true,
	},
	{
		ID:               "go",
		Family:           "go",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "Go",
		Extensions:       []string{".go"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_go.Language())
		},
		fragmentKinds:           kinds("func_literal", "function_declaration", "method_declaration"),
		excludeNestedBoundaries: true,
	},
	{
		ID:               "javascript",
		Family:           "javascript",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "JavaScript / JSX",
		Extensions:       []string{".cjs", ".js", ".jsx", ".mjs"},
		Shebangs:         []string{"node", "nodejs"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_javascript.Language())
		},
		fragmentKinds:           javascriptFunctions,
		excludeNestedBoundaries: true,
	},
	{
		ID:               "typescript",
		Family:           "typescript",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "TypeScript",
		Extensions:       []string{".cts", ".mts", ".ts"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
		},
		fragmentKinds:           javascriptFunctions,
		excludeNestedBoundaries: true,
	},
	{
		ID:               "tsx",
		Family:           "typescript",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "TypeScript / TSX",
		Extensions:       []string{".tsx"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
		},
		fragmentKinds:           javascriptFunctions,
		excludeNestedBoundaries: true,
	},
	{
		ID:               "python",
		Family:           "python",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "Python",
		Extensions:       []string{".py", ".pyi"},
		Shebangs:         []string{"python", "python3"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_python.Language())
		},
		fragmentKinds:           kinds("function_definition", "lambda"),
		excludeNestedBoundaries: true,
	},
	{
		ID:               "rust",
		Family:           "rust",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "Rust",
		Extensions:       []string{".rs"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_rust.Language())
		},
		fragmentKinds:           kinds("closure_expression", "function_item"),
		excludeNestedBoundaries: true,
	},
	{
		ID:               "swift",
		Family:           "swift",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "Swift",
		Extensions:       []string{".swift"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_swift.Language())
		},
		fragmentKinds: kinds(
			"deinit_declaration",
			"function_declaration",
			"init_declaration",
			"lambda_literal",
		),
		acceptBoundary:          acceptsSwiftBoundary,
		excludeNestedBoundaries: true,
	},
	{
		ID:               "zsh",
		Family:           "shell",
		ComparisonDomain: "code",
		FragmentKind:     "function",
		DisplayName:      "Zsh",
		Extensions:       []string{".zsh"},
		Shebangs:         []string{"zsh"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_zsh.Language())
		},
		fragmentKinds:           kinds("function_definition"),
		topLevelKinds:           kinds("program"),
		topLevelFragmentKind:    "script",
		excludeNestedBoundaries: true,
	},
	{
		ID:               "sql",
		Family:           "sql",
		ComparisonDomain: "sql-query",
		FragmentKind:     "query",
		DisplayName:      "SQL queries",
		Extensions:       []string{".sql"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_sql.Language())
		},
		fragmentKinds:  kinds("statement"),
		acceptBoundary: isSQLQueryStatement,
	},
	{
		ID:               "postgresql",
		Family:           "sql",
		ComparisonDomain: "sql-query",
		FragmentKind:     "query",
		DisplayName:      "PostgreSQL queries",
		Extensions:       []string{".sql"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_postgresql.Language())
		},
		fragmentKinds:  kinds("toplevel_stmt"),
		acceptBoundary: isPostgreSQLQueryStatement,
	},
}

func acceptsSwiftBoundary(node *tree_sitter.Node) bool {
	if node.Kind() == "lambda_literal" {
		parent := node.Parent()
		return parent != nil && parent.Parent() != nil
	}
	return node.ChildByFieldName("body") != nil
}

func isSQLQueryStatement(node *tree_sitter.Node) bool {
	parent := node.Parent()
	if parent == nil || parent.Kind() != "program" {
		return false
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		child := node.NamedChild(index)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "select", "set_operation", "insert", "update", "delete":
			return true
		}
	}
	return false
}

func isPostgreSQLQueryStatement(node *tree_sitter.Node) bool {
	parent := node.Parent()
	if parent == nil || parent.Kind() != "source_file" || node.NamedChildCount() != 1 {
		return false
	}
	statement := node.NamedChild(0)
	if statement == nil || statement.Kind() != "stmt" || statement.NamedChildCount() != 1 {
		return false
	}
	child := statement.NamedChild(0)
	if child == nil {
		return false
	}
	switch child.Kind() {
	case "SelectStmt", "InsertStmt", "UpdateStmt", "DeleteStmt":
		return true
	default:
		return false
	}
}

// Lookup returns one concrete grammar specification by ID.
func Lookup(id string) (Spec, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, spec := range specs {
		if spec.ID == id {
			return spec, true
		}
	}
	return Spec{}, false
}

// ResolveSelector expands one language ID or family into concrete grammar
// IDs in deterministic order.
func ResolveSelector(selector string) ([]string, bool) {
	selector = strings.ToLower(strings.TrimSpace(selector))
	resolved := make([]string, 0)
	for _, spec := range specs {
		if spec.ID == selector || spec.Family == selector {
			resolved = append(resolved, spec.ID)
		}
	}
	if len(resolved) == 0 {
		return nil, false
	}
	sort.Strings(resolved)
	return resolved, true
}

// ComparisonDomains returns the registered comparison domains in
// deterministic order.
func ComparisonDomains() []string {
	domains := make(map[string]struct{})
	for _, spec := range specs {
		domains[spec.ComparisonDomain] = struct{}{}
	}
	result := make([]string, 0, len(domains))
	for domain := range domains {
		result = append(result, domain)
	}
	sort.Strings(result)
	return result
}

// Detect returns the grammar associated with a file extension.
func Detect(path string) (Spec, bool) {
	extension := strings.ToLower(filepath.Ext(path))
	for _, spec := range specs {
		for _, candidate := range spec.Extensions {
			if extension == candidate {
				return spec, true
			}
		}
	}
	return Spec{}, false
}

// DetectWithSQLDialect returns the grammar associated with a file extension,
// selecting one explicit SQL parser for .sql files.
func DetectWithSQLDialect(path string, dialect string) (Spec, bool) {
	if strings.ToLower(filepath.Ext(path)) != ".sql" {
		return Detect(path)
	}
	target := "sql"
	if dialect == SQLDialectPostgreSQL {
		target = "postgresql"
	}
	for _, spec := range specs {
		if spec.ID == target {
			return spec, true
		}
	}
	return Spec{}, false
}

// SQLDialects returns the accepted SQL parser selections.
func SQLDialects() []string {
	return []string{SQLDialectGeneric, SQLDialectPostgreSQL}
}

// DetectShebang returns the grammar named by one bounded shebang line. It does
// not execute the interpreter or inspect any other source content.
func DetectShebang(line string) (Spec, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#!") || strings.ContainsRune(line, '\x00') {
		return Spec{}, false
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "#!")))
	if len(fields) == 0 {
		return Spec{}, false
	}
	interpreter := filepath.Base(fields[0])
	if interpreter == "env" {
		fields = fields[1:]
		for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
			if fields[0] == "-S" || fields[0] == "--split-string" {
				fields = fields[1:]
				break
			}
			fields = fields[1:]
		}
		if len(fields) == 0 || strings.Contains(fields[0], "=") {
			return Spec{}, false
		}
		interpreter = filepath.Base(fields[0])
	}
	for _, spec := range specs {
		for _, candidate := range spec.Shebangs {
			if interpreter == candidate {
				return spec, true
			}
		}
	}
	return Spec{}, false
}

// All returns all supported language specifications in display order.
func All() []Spec {
	result := make([]Spec, len(specs))
	for index, spec := range specs {
		spec.Extensions = append([]string(nil), spec.Extensions...)
		spec.Shebangs = append([]string(nil), spec.Shebangs...)
		spec.fragmentKinds = cloneKinds(spec.fragmentKinds)
		spec.topLevelKinds = cloneKinds(spec.topLevelKinds)
		result[index] = spec
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func cloneKinds(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for kind := range source {
		result[kind] = struct{}{}
	}
	return result
}

func kinds(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
