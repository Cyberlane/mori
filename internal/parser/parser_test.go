package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

func TestFileExtractsNamedAndAnonymousFunctions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "functions.js")
	content := `
export function named(value) {
  const clean = value.trim();
  return clean.includes("@") && clean.includes(".");
}

export const other = (input) => {
  const clean = input.trim();
  return clean.includes("@") && clean.includes(".");
};
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("JavaScript grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "functions.js",
		Language:    spec,
	}, 10)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(fragments) != 2 {
		t.Fatalf("fragments = %d, want 2", len(fragments))
	}
	if fragments[0].Location.Name != "named" {
		t.Errorf("first name = %q, want named", fragments[0].Location.Name)
	}
	if fragments[1].Location.Name != "other" {
		t.Errorf("second name = %q, want other", fragments[1].Location.Name)
	}
}

func TestFileWarnsAndSkipsInvalidFragment(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.py")
	if err := os.WriteFile(path, []byte("def broken(:\n  return True\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Python grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "broken.py",
		Language:    spec,
	}, 1)
	if len(fragments) != 0 {
		t.Fatalf("fragments = %d, want none", len(fragments))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one parse warning", warnings)
	}
}

func TestFileEnforcesReadLimit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "large.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Example() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Go grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "large.go",
		Language:    spec,
		MaxBytes:    8,
	}, 1)
	if len(fragments) != 0 {
		t.Fatalf("fragments = %d, want none", len(fragments))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one size warning", warnings)
	}
}

func TestFileRejectsIdentityChangeAfterDiscovery(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "changed.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc First() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(first): %v", err)
	}
	originalInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.WriteFile(path, []byte("package sample\nfunc Second() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(second): %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("Go grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "changed.go",
		Language:    spec,
		Info:        originalInfo,
	}, 1)
	if len(fragments) != 0 || len(warnings) != 1 {
		t.Fatalf("fragments/warnings = %d/%#v, want 0/one", len(fragments), warnings)
	}
	if strings.Contains(warnings[0].Message, path) {
		t.Fatalf("warning message %q contains absolute path", warnings[0].Message)
	}
}

func TestFileHandlesDeepSyntaxWithoutRecursiveWalk(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "deep.js")
	content := "function deep(value) { return " +
		strings.Repeat("(", 10_000) +
		"value" +
		strings.Repeat(")", 10_000) +
		"; }\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	spec, ok := language.Detect(path)
	if !ok {
		t.Fatal("JavaScript grammar not detected")
	}

	fragments, warnings := File(context.Background(), source.File{
		Path:        path,
		DisplayPath: "deep.js",
		Language:    spec,
	}, 1)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(fragments) != 1 {
		t.Fatalf("fragments = %d, want one", len(fragments))
	}
}

func TestTypeScriptDialectsExtractFunctions(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"typed.ts": `
export function present(value: string): boolean {
  return value.length > 0;
}
`,
		"view.tsx": `
export const View = () => {
  return <div>hello</div>;
};
`,
	}
	for name, content := range tests {
		name := name
		content := content
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			spec, ok := language.Detect(path)
			if !ok {
				t.Fatalf("%s grammar not detected", name)
			}
			fragments, warnings := File(context.Background(), source.File{
				Path:        path,
				DisplayPath: name,
				Language:    spec,
			}, 1)
			if len(warnings) != 0 {
				t.Fatalf("warnings = %#v", warnings)
			}
			if len(fragments) != 1 {
				t.Fatalf("fragments = %d, want 1", len(fragments))
			}
		})
	}
}
