// Package gdscript exposes Mori's pinned Tree-sitter GDScript grammar.
package gdscript

/*
#cgo CFLAGS: -std=c11 -fPIC
typedef struct TSLanguage TSLanguage;
const TSLanguage *tree_sitter_gdscript(void);
*/
import "C"

import "unsafe"

// Language returns the generated GDScript Tree-sitter grammar.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_gdscript())
}
