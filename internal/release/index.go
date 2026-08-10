package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var releaseVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type indexedArtifact struct {
	name string
	hash string
}

// IndexOptions identifies the already-built archives used to produce
// package-manager manifests. Generation never publishes to an external index.
type IndexOptions struct {
	DistDir string
	Version string
}

// GeneratePackageIndexes writes release-ready Homebrew, Scoop, and WinGet
// manifests whose URLs and SHA-256 values refer to immutable release assets.
func GeneratePackageIndexes(options IndexOptions) ([]string, error) {
	if !releaseVersionPattern.MatchString(options.Version) {
		return nil, fmt.Errorf("invalid release version %q", options.Version)
	}
	dist := options.DistDir
	if dist == "" {
		dist = "dist"
	}

	artifacts := make(map[string]indexedArtifact)
	for _, target := range []string{"darwin_amd64.tar.gz", "darwin_arm64.tar.gz", "linux_amd64.tar.gz", "linux_arm64.tar.gz", "windows_amd64.zip"} {
		name := "mori_" + options.Version + "_" + target
		content, err := os.ReadFile(filepath.Join(dist, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		digest := sha256.Sum256(content)
		artifacts[target] = indexedArtifact{name: name, hash: hex.EncodeToString(digest[:])}
	}

	baseURL := "https://github.com/Cyberlane/mori/releases/download/v" + options.Version + "/"
	formula := homebrewFormula(options.Version, baseURL, artifacts)
	scoop, err := scoopManifest(options.Version, baseURL, artifacts["windows_amd64.zip"])
	if err != nil {
		return nil, err
	}
	winget := wingetManifests(options.Version, baseURL, artifacts["windows_amd64.zip"])

	files := []struct {
		name    string
		content []byte
	}{
		{name: "mori.rb", content: []byte(formula)},
		{name: "mori.json", content: scoop},
		{name: "Cyberlane.Mori.yaml", content: []byte(winget[0])},
		{name: "Cyberlane.Mori.installer.yaml", content: []byte(winget[1])},
		{name: "Cyberlane.Mori.locale.en-US.yaml", content: []byte(winget[2])},
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		path := filepath.Join(dist, file.name)
		if err := os.WriteFile(path, file.content, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", file.name, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func homebrewFormula(version string, baseURL string, artifacts map[string]indexedArtifact) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, `class Mori < Formula
  desc "Explainable cross-language structural similarity"
  homepage "https://github.com/Cyberlane/mori"
  version %q
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url %q
      sha256 %q
    else
      url %q
      sha256 %q
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url %q
      sha256 %q
    else
      url %q
      sha256 %q
    end
  end

  def install
    bin.install Dir["mori_*/mori"].first
  end

  test do
    assert_match "mori #{version}", shell_output("#{bin}/mori version")
  end
end
`, version,
		baseURL+artifacts["darwin_arm64.tar.gz"].name, artifacts["darwin_arm64.tar.gz"].hash,
		baseURL+artifacts["darwin_amd64.tar.gz"].name, artifacts["darwin_amd64.tar.gz"].hash,
		baseURL+artifacts["linux_arm64.tar.gz"].name, artifacts["linux_arm64.tar.gz"].hash,
		baseURL+artifacts["linux_amd64.tar.gz"].name, artifacts["linux_amd64.tar.gz"].hash,
	)
	return builder.String()
}

func scoopManifest(version string, baseURL string, windows indexedArtifact) ([]byte, error) {
	manifest := struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
		License     string `json:"license"`
		URL         string `json:"url"`
		Hash        string `json:"hash"`
		ExtractDir  string `json:"extract_dir"`
		Bin         string `json:"bin"`
	}{
		Version: version, Description: "Explainable cross-language structural similarity",
		Homepage: "https://github.com/Cyberlane/mori", License: "MIT",
		URL: baseURL + windows.name, Hash: windows.hash,
		ExtractDir: "mori_" + version + "_windows_amd64", Bin: "mori.exe",
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Scoop manifest: %w", err)
	}
	return append(content, '\n'), nil
}

func wingetManifests(version string, baseURL string, windows indexedArtifact) [3]string {
	versionManifest := fmt.Sprintf(`# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.1.10.0.schema.json
PackageIdentifier: Cyberlane.Mori
PackageVersion: %s
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.10.0
`, version)
	installerManifest := fmt.Sprintf(`# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.1.10.0.schema.json
PackageIdentifier: Cyberlane.Mori
PackageVersion: %s
InstallerType: zip
NestedInstallerType: portable
Commands:
  - mori
Installers:
  - Architecture: x64
    NestedInstallerFiles:
      - RelativeFilePath: mori_%s_windows_amd64\mori.exe
        PortableCommandAlias: mori
    InstallerUrl: %s%s
    InstallerSha256: %s
ManifestType: installer
ManifestVersion: 1.10.0
`, version, version, baseURL, windows.name, strings.ToUpper(windows.hash))
	localeManifest := fmt.Sprintf(`# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.1.10.0.schema.json
PackageIdentifier: Cyberlane.Mori
PackageVersion: %s
PackageLocale: en-US
Publisher: Cyberlane
PackageName: Mori
License: MIT
ShortDescription: Explainable cross-language structural similarity
PackageUrl: https://github.com/Cyberlane/mori
LicenseUrl: https://github.com/Cyberlane/mori/blob/v%s/LICENSE
ManifestType: defaultLocale
ManifestVersion: 1.10.0
`, version, version)
	return [3]string{versionManifest, installerManifest, localeManifest}
}
