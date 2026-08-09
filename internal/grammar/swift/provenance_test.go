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
		"parser.c":             "9df63e0b6680f0b6cf1f1df613aaff2a7a4a3d9c9eb573b28b5d5c33fdaf7494",
		"scanner.c":            "380edc27e2020e5ba2d6415c9f6c0065965771d60138ae53372858e7b1f92e3b",
		"tree_sitter/parser.h": "a1f6ef161fbaf48a0e10fca90ef5290a062462b307b3898aa562993853b9f80a",
		"tree_sitter/array.h":  "4ff743903dc46f5db6aa54f31c6b4d160a8a9779e5b2ab1ee59ae7ebcd850ea1",
		"tree_sitter/alloc.h":  "253b44a7b4313a7afd0c505c2fc6e7ce4b8e78955ebf4be3ea000532ec060673",
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
