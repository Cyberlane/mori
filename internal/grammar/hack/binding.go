// Package hack exposes Mori's pinned Tree-sitter Hack grammar.
package hack

/*
#cgo CFLAGS: -std=c11 -fPIC
typedef struct TSLanguage TSLanguage;
const TSLanguage *tree_sitter_hack(void);
*/
import "C"

import "unsafe"

// Language returns the generated Hack Tree-sitter grammar.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_hack())
}
