package release

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cyberlane/mori/skills"
)

// SkillOptions identifies the portable Agent Skill release archive.
type SkillOptions struct {
	Root      string
	OutputDir string
	Version   string
	Timestamp time.Time
}

// PackageSkill writes Mori's portable Agent Skill as a deterministic ZIP.
func PackageSkill(options SkillOptions) (string, error) {
	if options.Root == "" {
		return "", errors.New("repository root is required")
	}
	if options.OutputDir == "" {
		return "", errors.New("output directory is required")
	}
	version := strings.TrimPrefix(options.Version, "v")
	if !safeIdentifier.MatchString(version) {
		return "", errors.New("version contains unsupported characters")
	}
	if options.Timestamp.IsZero() {
		return "", errors.New("timestamp is required")
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	archivePath := filepath.Join(
		options.OutputDir,
		fmt.Sprintf("%s_%s.zip", skills.ReviewSimilarityName, version),
	)
	files := []archiveFile{
		{
			sourcePath: filepath.Join(
				options.Root,
				"skills",
				skills.ReviewSimilarityName,
				"SKILL.md",
			),
			name: filepath.ToSlash(filepath.Join(skills.ReviewSimilarityName, "SKILL.md")),
			mode: 0o644,
		},
		{
			sourcePath: filepath.Join(
				options.Root,
				"skills",
				skills.ReviewSimilarityName,
				"agents",
				"openai.yaml",
			),
			name: filepath.ToSlash(filepath.Join(
				skills.ReviewSimilarityName,
				"agents",
				"openai.yaml",
			)),
			mode: 0o644,
		},
	}
	for index := range files {
		info, err := os.Lstat(files[index].sourcePath)
		if err != nil {
			return "", fmt.Errorf("package %s: %w", files[index].sourcePath, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("package %s: input is not a regular file", files[index].sourcePath)
		}
		relative, err := filepath.Rel(options.Root, files[index].sourcePath)
		if err != nil {
			return "", fmt.Errorf("resolve package input %s: %w", files[index].sourcePath, err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("package input is outside repository root: %s", files[index].sourcePath)
		}
		files[index].info = info
	}
	if err := rejectOutputCollision(archivePath, files); err != nil {
		return "", err
	}
	if err := writeAtomically(archivePath, func(writer io.Writer) error {
		return writeZip(writer, files, options.Timestamp)
	}); err != nil {
		return "", err
	}
	return archivePath, nil
}
