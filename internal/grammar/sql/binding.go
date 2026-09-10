// Package sql exposes Mori's pinned Tree-sitter SQL grammar.
package sql

/*
#cgo CFLAGS: -std=c11 -fPIC
typedef struct TSLanguage TSLanguage;
const TSLanguage *tree_sitter_sql(void);
*/
import "C"

import "unsafe"

// Language returns the generated SQL Tree-sitter grammar.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_sql())
}
