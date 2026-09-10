package cachefs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateCreationIsExclusive(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := Mkdir(root); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(root)
	if err != nil || !Private(root, info) {
		t.Fatalf("directory is not private: %v", err)
	}
	path := filepath.Join(root, "key")
	file, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err = os.Lstat(path)
	if err != nil || !Private(path, info) {
		t.Fatalf("file is not private: %v", err)
	}
	if file, err := Create(path); !errors.Is(err, os.ErrExist) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("exclusive creation: %v", err)
	}
}
