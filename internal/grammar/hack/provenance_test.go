package hack

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
		"parser.c":             "d3c9389eb2ccaf13de95c0465e71306a187886300507c8812bf69101978aba5a",
		"scanner.c":            "6d003ff56371b4ee31252ecfab216dccd2ce22ddcbafb482ca167fca77d5b199",
		"tree_sitter/parser.h": "ab104936984904469572a4e868149f7a22fb2929347f837ae6a1f9b790f1b173",
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
