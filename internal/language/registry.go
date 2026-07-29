// Package language owns supported Tree-sitter grammars and fragment boundaries.
package language

import (
	"path/filepath"
	"sort"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Spec describes a parser grammar and the nodes that form comparison units.
type Spec struct {
	ID            string
	DisplayName   string
	Extensions    []string
	newLanguage   func() *tree_sitter.Language
	functionKinds map[string]struct{}
}

// NewLanguage returns a Tree-sitter language wrapper for this grammar.
func (s Spec) NewLanguage() *tree_sitter.Language {
	return s.newLanguage()
}

// IsFunction reports whether kind is a function-like comparison boundary.
func (s Spec) IsFunction(kind string) bool {
	_, ok := s.functionKinds[kind]
	return ok
}

var javascriptFunctions = kinds(
	"arrow_function",
	"function_declaration",
	"function_expression",
	"generator_function",
	"generator_function_declaration",
	"method_definition",
)

var specs = []Spec{
	{
		ID:          "go",
		DisplayName: "Go",
		Extensions:  []string{".go"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_go.Language())
		},
		functionKinds: kinds("func_literal", "function_declaration", "method_declaration"),
	},
	{
		ID:          "javascript",
		DisplayName: "JavaScript / JSX",
		Extensions:  []string{".cjs", ".js", ".jsx", ".mjs"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_javascript.Language())
		},
		functionKinds: javascriptFunctions,
	},
	{
		ID:          "typescript",
		DisplayName: "TypeScript",
		Extensions:  []string{".cts", ".mts", ".ts"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
		},
		functionKinds: javascriptFunctions,
	},
	{
		ID:          "tsx",
		DisplayName: "TypeScript / TSX",
		Extensions:  []string{".tsx"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
		},
		functionKinds: javascriptFunctions,
	},
	{
		ID:          "python",
		DisplayName: "Python",
		Extensions:  []string{".py", ".pyi"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_python.Language())
		},
		functionKinds: kinds("function_definition", "lambda"),
	},
	{
		ID:          "rust",
		DisplayName: "Rust",
		Extensions:  []string{".rs"},
		newLanguage: func() *tree_sitter.Language {
			return tree_sitter.NewLanguage(tree_sitter_rust.Language())
		},
		functionKinds: kinds("closure_expression", "function_item"),
	},
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

// All returns all supported language specifications in display order.
func All() []Spec {
	result := make([]Spec, len(specs))
	for index, spec := range specs {
		spec.Extensions = append([]string(nil), spec.Extensions...)
		spec.functionKinds = cloneKinds(spec.functionKinds)
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
