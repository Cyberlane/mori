// Package swift exposes Mori's pinned Tree-sitter Swift grammar.
package swift

/*
#cgo CFLAGS: -std=c11 -fPIC
typedef struct TSLanguage TSLanguage;
const TSLanguage *tree_sitter_swift(void);
*/
import "C"

import "unsafe"

// Language returns the generated Swift Tree-sitter grammar.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_swift())
}
