package sql

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
		"LICENSE":              "3b3e4e4252d5d7d1c18e1257005f23242bf0580ad619204fd093c99b8c56748d",
		"grammar.js":           "40d16a63a468f0bac2aa67491cb90c8dc7cf5dd1f5ef30ea0814b1c1234f59ed",
		"parser.c":             "a5e3de0fc435d65d4597c565afcb856b7d86ea88fa6f6e7c5500540055c18727",
		"scanner.c":            "4d09c84073f20be5fff2a0bee49dbdeafdf2428913cb6543e2986e07a246fc86",
		"tree-sitter.json":     "785766c0d78e9534330540f7a0dcc64e601129973b989c223d3effb31d846a90",
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
