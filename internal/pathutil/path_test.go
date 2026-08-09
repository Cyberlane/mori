package pathutil

import (
	"path/filepath"
	"testing"
)

func TestWithin(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "base", path: base, want: true},
		{name: "descendant", path: filepath.Join(base, "child", "file.go"), want: true},
		{name: "cleaned descendant", path: filepath.Join(base, "child", "..", "file.go"), want: true},
		{name: "parent", path: filepath.Dir(base), want: false},
		{name: "sibling prefix", path: base + "-other", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Within(base, test.path); got != test.want {
				t.Fatalf("Within(%q, %q) = %t, want %t", base, test.path, got, test.want)
			}
		})
	}
}
