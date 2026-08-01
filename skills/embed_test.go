package skills

import (
	"io/fs"
	"strings"
	"testing"
)

func TestReviewSimilarityPackage(t *testing.T) {
	t.Parallel()

	packageFS, err := ReviewSimilarity()
	if err != nil {
		t.Fatalf("ReviewSimilarity: %v", err)
	}
	wantFiles := map[string]bool{
		"SKILL.md":           false,
		"agents/openai.yaml": false,
	}
	if err := fs.WalkDir(packageFS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			if _, ok := wantFiles[path]; !ok {
				t.Errorf("unexpected embedded file %q", path)
			} else {
				wantFiles[path] = true
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	for path, found := range wantFiles {
		if !found {
			t.Errorf("embedded file %q is missing", path)
		}
	}

	skill, err := fs.ReadFile(packageFS, "SKILL.md")
	if err != nil {
		t.Fatalf("ReadFile(SKILL.md): %v", err)
	}
	text := string(skill)
	for _, expected := range []string{
		"name: " + ReviewSimilarityName,
		"schema_version",
		"never as proof of equivalent behavior",
		"Do not refactor, delete, or consolidate code solely because Mori reported a",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("SKILL.md missing %q", expected)
		}
	}
	if strings.Contains(text, "TODO") {
		t.Fatal("SKILL.md contains an unresolved TODO")
	}

	metadata, err := fs.ReadFile(packageFS, "agents/openai.yaml")
	if err != nil {
		t.Fatalf("ReadFile(agents/openai.yaml): %v", err)
	}
	if !strings.Contains(string(metadata), "$"+ReviewSimilarityName) {
		t.Fatal("agents/openai.yaml default prompt does not name the skill")
	}
}
