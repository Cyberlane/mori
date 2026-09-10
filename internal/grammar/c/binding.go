// Package c exposes Mori's pinned Tree-sitter C grammar.
package c

/*
#cgo CFLAGS: -std=c11 -fPIC
typedef struct TSLanguage TSLanguage;
const TSLanguage *tree_sitter_c(void);
*/
import "C"

import "unsafe"

// Language returns the generated C Tree-sitter grammar.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_c())
}
