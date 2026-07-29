package language

import (
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestEveryGrammarHasCompatibleABI(t *testing.T) {
	t.Parallel()

	for _, spec := range All() {
		spec := spec
		t.Run(spec.ID, func(t *testing.T) {
			t.Parallel()

			parser := tree_sitter.NewParser()
			defer parser.Close()
			if err := parser.SetLanguage(spec.NewLanguage()); err != nil {
				t.Fatalf("SetLanguage(%s): %v", spec.ID, err)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"main.go":       "go",
		"view.JSX":      "javascript",
		"worker.mts":    "typescript",
		"component.tsx": "tsx",
		"module.pyi":    "python",
		"lib.rs":        "rust",
	}
	for path, expected := range tests {
		spec, ok := Detect(path)
		if !ok {
			t.Fatalf("Detect(%q) returned unsupported", path)
		}
		if spec.ID != expected {
			t.Errorf("Detect(%q) ID = %q, want %q", path, spec.ID, expected)
		}
	}

	if _, ok := Detect("README.md"); ok {
		t.Fatal("Detect(README.md) unexpectedly returned a language")
	}
}

func TestAllReturnsIndependentSpecifications(t *testing.T) {
	t.Parallel()

	first := All()
	first[0].Extensions[0] = ".mutated"
	first[0].functionKinds["mutated"] = struct{}{}

	second := All()
	if second[0].Extensions[0] == ".mutated" || second[0].IsFunction("mutated") {
		t.Fatal("mutating All result changed the registry")
	}
}
