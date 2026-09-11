package java

/*
#cgo CFLAGS: -std=c11 -fPIC
typedef struct TSLanguage TSLanguage;
const TSLanguage *tree_sitter_java(void);
*/
import "C"
import "unsafe"

func Language() unsafe.Pointer { return unsafe.Pointer(C.tree_sitter_java()) }
