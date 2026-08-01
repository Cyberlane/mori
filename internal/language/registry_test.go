package language

import (
	"testing"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestEveryGrammarHasCompatibleABI(t *testing.T) {
	t.Parallel()

	expectedABI := map[string]uint32{
		"go":         15,
		"javascript": 15,
		"python":     15,
		"rust":       15,
		"tsx":        14,
		"typescript": 14,
	}

	for _, spec := range All() {
		spec := spec
		t.Run(spec.ID, func(t *testing.T) {
			t.Parallel()

			language := spec.NewLanguage()
			if got, want := language.AbiVersion(), expectedABI[spec.ID]; got != want {
				t.Fatalf("%s ABI = %d, want %d", spec.ID, got, want)
			}
			if got := language.AbiVersion(); got < tree_sitter.MIN_COMPATIBLE_LANGUAGE_VERSION ||
				got > tree_sitter.LANGUAGE_VERSION {
				t.Fatalf(
					"%s ABI = %d, supported range is %d-%d",
					spec.ID,
					got,
					tree_sitter.MIN_COMPATIBLE_LANGUAGE_VERSION,
					tree_sitter.LANGUAGE_VERSION,
				)
			}
			parser := tree_sitter.NewParser()
			defer parser.Close()
			if err := parser.SetLanguage(language); err != nil {
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
