package c

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"
)

func TestGeneratedSourceChecksums(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		"LICENSE":              "2e0110e07abef7c2548b26ec9d6969775617ca539a0dc8dbeeb14d6452c711d1",
		"grammar.js":           "c67fd24cea0ea4d170c5b16577ad0675c04455a605da7505884f583a01475700",
		"parser.c":             "db443d113b0c66d0f3ba83ecaa7536fac818a1f49bb60e30ab208f3805fa0ae6",
		"tree-sitter.json":     "9a823f24385fbb5fa06e52c258c726449c4e417a4476306589b6dcfaf9b831ee",
		"tree_sitter/alloc.h":  "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea",
		"tree_sitter/array.h":  "5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94",
		"tree_sitter/parser.h": "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2",
	}
	for path, want := range expected {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("Open(%s): %v", path, err)
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			t.Fatalf("hash %s: %v", path, copyErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", path, closeErr)
		}
		if got := hex.EncodeToString(digest.Sum(nil)); got != want {
			t.Fatalf("%s SHA-256 = %s, want %s", path, got, want)
		}
	}
}
