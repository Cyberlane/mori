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

func TestIsRootedAcrossPlatforms(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		path string
		want bool
	}{
		{"/tmp/tests/project/main.ts", true}, {`C:\tests\project\main.ts`, true}, {"C:/tests/project/main.ts", true}, {`\\server\tests\main.ts`, true}, {`\tests\main.ts`, true}, {"//server/tests/main.ts", true}, {"C:tests/main.ts", true}, {"tests/main.ts", false}, {"packages/runtime-test/src/main.ts", false}, {"./tests/main.ts", false}, {"../tests/main.ts", false}, {"", false}, {"1:tests/main.ts", false},
	} {
		if got := IsRooted(tt.path); got != tt.want {
			t.Errorf("IsRooted(%q)=%t want %t", tt.path, got, tt.want)
		}
	}
}
