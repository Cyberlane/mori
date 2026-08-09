// Package postgresql exposes Mori's pinned Tree-sitter PostgreSQL grammar.
package postgresql

/*
#cgo CFLAGS: -std=c11 -fPIC
typedef struct TSLanguage TSLanguage;
const TSLanguage *tree_sitter_postgres(void);
*/
import "C"

import "unsafe"

// Language returns the generated PostgreSQL Tree-sitter grammar.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_postgres())
}
