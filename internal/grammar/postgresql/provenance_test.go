package postgresql

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
		"parser.c":             "ba8c9b24ae61c1672cbcbe553eafc9815f19e132f0c1cfe7600e3dd4c1a8fd66",
		"scanner.c":            "4d7872a6bb2126e206a7aa0730b7f272b41d9e75c2d11af274fc4db2cd7c3352",
		"tree_sitter/parser.h": "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2",
		"tree_sitter/array.h":  "31e60a1bff6f715afacce03b5b70efe42b58371b4f9595dd4af52a577ff9608c",
		"tree_sitter/alloc.h":  "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea",
		"LICENSE":              "b336477d5469bf335e7d173814e1da57d4f13c23968959a71523668ac4a9a6c2",
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
