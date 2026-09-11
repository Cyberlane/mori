package golang

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
)

func TestGeneratedSourceChecksums(t *testing.T) {
	for path, want := range map[string]string{
		"LICENSE":              "2e0110e07abef7c2548b26ec9d6969775617ca539a0dc8dbeeb14d6452c711d1",
		"grammar.js":           "cca1a66bf2fd6671b48dd1f0d92709245c55f26adda32d4b0a24207ae3e332d6",
		"parser.c":             "d0b1955e4a50b2c250c859fdcd4432c5d5e93f575ac6887f66002246bf6e989e",
		"tree-sitter.json":     "80d8a24648d35abb30a3cb2dbcc3a1af3919f274bcabcc277c0d9535d60abf9e",
		"tree_sitter/parser.h": "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2",
		"tree_sitter/alloc.h":  "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea",
		"tree_sitter/array.h":  "5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94",
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
