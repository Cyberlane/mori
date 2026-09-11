package java

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
)

func TestGeneratedSourceChecksums(t *testing.T) {
	for path, want := range map[string]string{
		"LICENSE":              "52ed137b039cd9c46409bc22e89938af911c95b157feae2d040b51e6084369a7",
		"binding.go":           "4970ba37369918114850fc2d8cbc93b80e9857bc25dfb41b425475171e9095f4",
		"grammar.js":           "42f34b25520c85d774558e0f8c3f91fd4cb87fd385b342ebf0e299ec20e1d46e",
		"parser.c":             "8695f9e55fac0e7bf24956af304d9746ce053aa108811216633a9c311dfe3934",
		"tree-sitter.json":     "5b8f0cb5514a4a04720beba0e5991c48837844fe0ce2ba2d02ec7e6a44a344d8",
		"tree_sitter/alloc.h":  "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea",
		"tree_sitter/array.h":  "5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94",
		"tree_sitter/parser.h": "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2",
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != want {
			t.Fatalf("%s checksum %s, want %s", path, got, want)
		}
	}
}
