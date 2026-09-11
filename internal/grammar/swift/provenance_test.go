package swift

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
		"grammar.js":           "e8a81cb8bbd7ee8ab4652ecafe7849991377ae63fd5d06c29777a67f19f54da9",
		"parser.c":             "cde85fddaa1f0579d840abe589538735eec1e7a917561eeed77d3b20843baf03",
		"scanner.c":            "380edc27e2020e5ba2d6415c9f6c0065965771d60138ae53372858e7b1f92e3b",
		"tree_sitter/parser.h": "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2",
		"tree_sitter/array.h":  "5bdf6ed1a78e3409fd443e085ca967a64c188a5d082aaf7f819bccd53a471c94",
		"tree_sitter/alloc.h":  "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea",
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
