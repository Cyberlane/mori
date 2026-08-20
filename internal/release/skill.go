package release

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
	files, err := skillFiles(options.Root)
	if err != nil {
		return "", err
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

func skillFiles(root string) ([]archiveFile, error) {
	skillRoot := filepath.Join(root, "skills", skills.ReviewSimilarityName)
	rootInfo, err := os.Lstat(skillRoot)
	if err != nil {
		return nil, fmt.Errorf("package %s: %w", skillRoot, err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, fmt.Errorf("package %s: input is not a real directory", skillRoot)
	}

	files := make([]archiveFile, 0, 4)
	err = filepath.WalkDir(skillRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("package %s: input contains a symlink", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("package %s: input is not a regular file", path)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect package input %s: %w", path, err)
		}
		relative, err := filepath.Rel(skillRoot, path)
		if err != nil {
			return fmt.Errorf("resolve package input %s: %w", path, err)
		}
		files = append(files, archiveFile{
			sourcePath: path,
			name: filepath.ToSlash(filepath.Join(
				skills.ReviewSimilarityName,
				relative,
			)),
			mode: 0o644,
			info: info,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk skill package: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}
