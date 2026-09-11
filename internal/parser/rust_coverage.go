package parser

import ts "github.com/tree-sitter/go-tree-sitter"

// Rust item macros can emit functions that are not syntax-tree boundaries.
// Report their opacity; do not guess a macro DSL or execute expansion.
func hasOpaqueRustItemMacro(root *ts.Node) bool {
	cursor := root.Walk()
	defer cursor.Close()
	for {
		current := cursor.Node()
		if current.Kind() == "macro_invocation" {
			parent := current.Parent()
			if parent != nil && parent.Kind() == "expression_statement" {
				parent = parent.Parent()
			}
			if parent != nil && (parent.Kind() == "source_file" || parent.Kind() == "declaration_list") {
				return true
			}
		}
		if cursor.GotoFirstChild() {
			continue
		}
		for {
			if cursor.GotoNextSibling() {
				break
			}
			if !cursor.GotoParent() {
				return false
			}
		}
	}
}
