package javascript

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
)

func TestGeneratedSourceChecksums(t *testing.T) {
	for path, want := range map[string]string{
		"LICENSE":              "2e0110e07abef7c2548b26ec9d6969775617ca539a0dc8dbeeb14d6452c711d1",
		"binding.go":           "36cdd8dbb3fffbcf4862a12ea84fe0fe792bfa8be6554030d7d5442e447c6fc2",
		"grammar.js":           "3410b7c45c233c7d3899daa91de00e2a7db7fb1810f2dd45eec9a4cc6fa01f21",
		"parser.c":             "87fcf8b68b32a91eabdf414ae50916af88797e00bc4ce33bcba1749bd1dfcd60",
		"scanner.c":            "b3d3f64284d97bf80749c026862427782cf7ecc0b7dc094e6698ab311c9a42c7",
		"tree-sitter.json":     "43f1abe79d69601f9699d292ccbdcf04c8c836911984bd6e466c8368711a1d65",
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
