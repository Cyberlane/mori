package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestPackageIsByteForByteDeterministic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"README.md":              "readme",
		"LICENSE":                "license",
		"THIRD_PARTY_NOTICES.md": "notices",
		"mori":                   "binary",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	options := Options{
		BinaryPath: filepath.Join(root, "mori"),
		Root:       root,
		OutputDir:  filepath.Join(root, "dist"),
		Version:    "1.2.3",
		GOOS:       "linux",
		GOARCH:     "amd64",
		Timestamp:  time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
	}

	firstPath, err := Package(options)
	if err != nil {
		t.Fatalf("first Package: %v", err)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("ReadFile(first): %v", err)
	}
	secondPath, err := Package(options)
	if err != nil {
		t.Fatalf("second Package: %v", err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("ReadFile(second): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical package inputs produced different archive bytes")
	}
}

func TestPackageWindowsArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"README.md":              "readme",
		"LICENSE":                "license",
		"THIRD_PARTY_NOTICES.md": "notices",
		"mori.exe":               "binary",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	archivePath, err := Package(Options{
		BinaryPath: filepath.Join(root, "mori.exe"),
		Root:       root,
		OutputDir:  filepath.Join(root, "dist"),
		Version:    "v1.2.3",
		GOOS:       "windows",
		GOARCH:     "amd64",
		Timestamp:  time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Package: %v", err)
	}
	if filepath.Base(archivePath) != "mori_1.2.3_windows_amd64.zip" {
		t.Fatalf("archive = %q", archivePath)
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	sort.Strings(names)
	want := []string{
		"mori_1.2.3_windows_amd64/LICENSE",
		"mori_1.2.3_windows_amd64/README.md",
		"mori_1.2.3_windows_amd64/THIRD_PARTY_NOTICES.md",
		"mori_1.2.3_windows_amd64/mori.exe",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("archive names = %#v, want %#v", names, want)
	}
}

func TestPackageRejectsUnsafeVersion(t *testing.T) {
	t.Parallel()

	_, err := Package(Options{
		BinaryPath: "mori",
		Root:       ".",
		OutputDir:  "dist",
		Version:    "../../escape",
		GOOS:       "linux",
		GOARCH:     "amd64",
		Timestamp:  time.Now(),
	})
	if err == nil {
		t.Fatal("Package returned nil for an unsafe version")
	}
}

func TestPackageRejectsOutputAliasingInput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"README.md":              "readme",
		"LICENSE":                "license",
		"THIRD_PARTY_NOTICES.md": "notices",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	archivePath := filepath.Join(root, "mori_1.2.3_linux_amd64.tar.gz")
	original := []byte("binary that must not be truncated")
	if err := os.WriteFile(archivePath, original, 0o700); err != nil {
		t.Fatalf("WriteFile(binary): %v", err)
	}

	_, err := Package(Options{
		BinaryPath: archivePath,
		Root:       root,
		OutputDir:  root,
		Version:    "1.2.3",
		GOOS:       "linux",
		GOARCH:     "amd64",
		Timestamp:  time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("Package returned nil for output/input alias")
	}
	after, readErr := os.ReadFile(archivePath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("aliased binary changed after rejected package")
	}
}

func TestPackageReplacesOutputSymlinkWithoutTouchingTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"README.md":              "readme",
		"LICENSE":                "license",
		"THIRD_PARTY_NOTICES.md": "notices",
		"mori":                   "binary",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	victim := filepath.Join(root, "victim")
	if err := os.WriteFile(victim, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("WriteFile(victim): %v", err)
	}
	archivePath := filepath.Join(root, "mori_1.2.3_linux_amd64.tar.gz")
	if err := os.Symlink(victim, archivePath); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}

	if _, err := Package(Options{
		BinaryPath: filepath.Join(root, "mori"),
		Root:       root,
		OutputDir:  root,
		Version:    "1.2.3",
		GOOS:       "linux",
		GOARCH:     "amd64",
		Timestamp:  time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Package: %v", err)
	}
	victimContent, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("ReadFile(victim): %v", err)
	}
	if string(victimContent) != "keep me" {
		t.Fatalf("victim = %q, want unchanged", victimContent)
	}
	if info, err := os.Lstat(archivePath); err != nil {
		t.Fatalf("Lstat(archive): %v", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("archive path is still a symlink")
	}
}

func TestPackageTarGzipArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"README.md":              "readme",
		"LICENSE":                "license",
		"THIRD_PARTY_NOTICES.md": "notices",
		"mori":                   "binary",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	archivePath, err := Package(Options{
		BinaryPath: filepath.Join(root, "mori"),
		Root:       root,
		OutputDir:  filepath.Join(root, "dist"),
		Version:    "1.2.3",
		GOOS:       "linux",
		GOARCH:     "arm64",
		Timestamp:  time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Package: %v", err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	names := make([]string, 0, 4)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("Next: %v", err)
		}
		names = append(names, header.Name)
	}
	sort.Strings(names)
	want := []string{
		"mori_1.2.3_linux_arm64/LICENSE",
		"mori_1.2.3_linux_arm64/README.md",
		"mori_1.2.3_linux_arm64/THIRD_PARTY_NOTICES.md",
		"mori_1.2.3_linux_arm64/mori",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("archive names = %#v, want %#v", names, want)
	}
}
