package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePackageIndexes(t *testing.T) {
	t.Parallel()
	dist := t.TempDir()
	version := "1.2.3"
	for _, target := range []string{"darwin_amd64.tar.gz", "darwin_arm64.tar.gz", "linux_amd64.tar.gz", "linux_arm64.tar.gz", "windows_amd64.zip"} {
		name := "mori_" + version + "_" + target
		if err := os.WriteFile(filepath.Join(dist, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := GeneratePackageIndexes(IndexOptions{DistDir: dist, Version: version})
	if err != nil {
		t.Fatalf("GeneratePackageIndexes: %v", err)
	}
	if len(paths) != 5 {
		t.Fatalf("paths = %#v", paths)
	}

	formula := readIndexFixture(t, dist, "mori.rb")
	for _, expected := range []string{"class Mori < Formula", `version "1.2.3"`, "darwin_arm64.tar.gz", "linux_amd64.tar.gz", "sha256"} {
		if !strings.Contains(formula, expected) {
			t.Errorf("formula does not contain %q", expected)
		}
	}

	scoop := readIndexFixture(t, dist, "mori.json")
	var manifest map[string]any
	if err := json.Unmarshal([]byte(scoop), &manifest); err != nil {
		t.Fatalf("Scoop JSON: %v", err)
	}
	if manifest["version"] != version || manifest["bin"] != "mori.exe" {
		t.Fatalf("Scoop manifest = %#v", manifest)
	}

	installer := readIndexFixture(t, dist, "Cyberlane.Mori.installer.yaml")
	for _, expected := range []string{"NestedInstallerType: portable", "PortableCommandAlias: mori", "InstallerSha256:", "mori_1.2.3_windows_amd64\\mori.exe"} {
		if !strings.Contains(installer, expected) {
			t.Errorf("WinGet installer does not contain %q", expected)
		}
	}
}

func TestGeneratePackageIndexesRejectsInvalidVersionAndMissingArchive(t *testing.T) {
	t.Parallel()
	if _, err := GeneratePackageIndexes(IndexOptions{DistDir: t.TempDir(), Version: "v1.2.3"}); err == nil {
		t.Fatal("accepted version with v prefix")
	}
	if _, err := GeneratePackageIndexes(IndexOptions{DistDir: t.TempDir(), Version: "1.2.3"}); err == nil {
		t.Fatal("accepted missing release archives")
	}
}

func readIndexFixture(t *testing.T, root string, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
