package release

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Cyberlane/mori/skills"
)

func TestPackageSkillIsDeterministicAndPortable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillRoot := filepath.Join(root, "skills", skills.ReviewSimilarityName)
	if err := os.MkdirAll(filepath.Join(skillRoot, "agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for path, content := range map[string]string{
		filepath.Join(skillRoot, "SKILL.md"):              "skill",
		filepath.Join(skillRoot, "agents", "openai.yaml"): "metadata",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
	options := SkillOptions{
		Root:      root,
		OutputDir: filepath.Join(root, "dist"),
		Version:   "v0.2.0",
		Timestamp: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}

	firstPath, err := PackageSkill(options)
	if err != nil {
		t.Fatalf("PackageSkill(first): %v", err)
	}
	if filepath.Base(firstPath) != "mori-review-similarity_0.2.0.zip" {
		t.Fatalf("archive = %q", firstPath)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("ReadFile(first): %v", err)
	}
	secondPath, err := PackageSkill(options)
	if err != nil {
		t.Fatalf("PackageSkill(second): %v", err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("ReadFile(second): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical skill inputs produced different archive bytes")
	}

	reader, err := zip.OpenReader(firstPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
		if file.Mode().Perm() != 0o644 {
			t.Errorf("%s mode = %o, want 644", file.Name, file.Mode().Perm())
		}
	}
	sort.Strings(names)
	want := []string{
		"mori-review-similarity/SKILL.md",
		"mori-review-similarity/agents/openai.yaml",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("archive names = %#v, want %#v", names, want)
	}
}

func TestPackageSkillRejectsUnsafeVersion(t *testing.T) {
	t.Parallel()

	_, err := PackageSkill(SkillOptions{
		Root:      ".",
		OutputDir: "dist",
		Version:   "../../escape",
		Timestamp: time.Now(),
	})
	if err == nil {
		t.Fatal("PackageSkill returned nil for an unsafe version")
	}
}
