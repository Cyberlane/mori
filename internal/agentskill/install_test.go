package agentskill

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/skills"
)

func TestInstallNewAndCurrent(t *testing.T) {
	t.Parallel()

	parent := filepath.Join(t.TempDir(), "nested", "skills")
	first, err := Install(parent, false)
	if err != nil {
		t.Fatalf("Install(new): %v", err)
	}
	if first.Status != StatusInstalled {
		t.Fatalf("new status = %q, want %q", first.Status, StatusInstalled)
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatalf("EvalSymlinks(parent): %v", err)
	}
	if first.Path != filepath.Join(resolvedParent, skills.ReviewSimilarityName) {
		t.Fatalf("new path = %q", first.Path)
	}
	assertInstalledMatchesEmbedded(t, first.Path)

	second, err := Install(parent, false)
	if err != nil {
		t.Fatalf("Install(current): %v", err)
	}
	if second.Status != StatusCurrent || second.Path != first.Path {
		t.Fatalf("current result = %#v", second)
	}
}

func TestInspectIsReadOnlyAndReportsDrift(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(t.TempDir(), "missing", "skills")
	result, err := Inspect(parent)
	if err != nil || result.Status != StatusMissing {
		t.Fatalf("Inspect(missing) = %#v, %v", result, err)
	}
	if _, err := os.Lstat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Inspect created parent: %v", err)
	}
	installed, err := Install(parent, false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	result, err = Inspect(parent)
	if err != nil || result.Status != StatusCurrent {
		t.Fatalf("Inspect(current) = %#v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(installed.Path, "SKILL.md"), []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	result, err = Inspect(parent)
	if err != nil || result.Status != StatusDifferent {
		t.Fatalf("Inspect(different) = %#v, %v", result, err)
	}
}

func TestInstallRejectsDifferentContentAndPreservesBackup(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	result, err := Install(parent, false)
	if err != nil {
		t.Fatalf("Install(new): %v", err)
	}
	skillPath := filepath.Join(result.Path, "SKILL.md")
	custom := []byte("custom skill\n")
	if err := os.WriteFile(skillPath, custom, 0o644); err != nil {
		t.Fatalf("WriteFile(custom): %v", err)
	}

	if _, err := Install(parent, false); !errors.Is(err, ErrDifferent) {
		t.Fatalf("Install(different) error = %v, want ErrDifferent", err)
	}
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile(custom): %v", err)
	}
	if string(content) != string(custom) {
		t.Fatal("rejected install changed the customized skill")
	}

	replaced, err := Install(parent, true)
	if err != nil {
		t.Fatalf("Install(replace): %v", err)
	}
	if replaced.Status != StatusReplaced || replaced.BackupPath == "" {
		t.Fatalf("replacement result = %#v", replaced)
	}
	backup, err := os.ReadFile(filepath.Join(replaced.BackupPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(backup): %v", err)
	}
	if string(backup) != string(custom) {
		t.Fatal("replacement backup did not preserve the customized skill")
	}
	assertInstalledMatchesEmbedded(t, replaced.Path)
}

func TestInstallTreatsExtraContentAsDifferent(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	result, err := Install(parent, false)
	if err != nil {
		t.Fatalf("Install(new): %v", err)
	}
	if err := os.WriteFile(filepath.Join(result.Path, "notes.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile(extra): %v", err)
	}
	if _, err := Install(parent, false); !errors.Is(err, ErrDifferent) {
		t.Fatalf("Install(extra) error = %v, want ErrDifferent", err)
	}
}

func TestInstallRejectsSymlinkPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realParent := filepath.Join(root, "real")
	if err := os.Mkdir(realParent, 0o755); err != nil {
		t.Fatalf("Mkdir(real): %v", err)
	}
	linkedParent := filepath.Join(root, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}
	if _, err := Install(linkedParent, false); err == nil {
		t.Fatal("Install accepted a symlinked parent")
	}

	destination := filepath.Join(realParent, skills.ReviewSimilarityName)
	if err := os.Symlink(root, destination); err != nil {
		t.Fatalf("Symlink(destination): %v", err)
	}
	if _, err := Install(realParent, false); err == nil {
		t.Fatal("Install accepted a symlinked skill destination")
	}
}

func assertInstalledMatchesEmbedded(t *testing.T, destination string) {
	t.Helper()

	files, directories, err := packageContents()
	if err != nil {
		t.Fatalf("packageContents: %v", err)
	}
	equal, err := equalPackage(destination, files, directories)
	if err != nil {
		t.Fatalf("equalPackage: %v", err)
	}
	if !equal {
		t.Fatal("installed skill differs from the embedded package")
	}
}
