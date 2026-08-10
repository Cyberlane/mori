// Package agentskill installs Mori's embedded Agent Skill safely and
// deterministically.
package agentskill

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/skills"
)

// ErrDifferent reports that an installed skill contains different or extra
// content and requires explicit replacement.
var ErrDifferent = errors.New("installed skill has different contents")

// Name is Mori's official structural-review Agent Skill name.
const Name = skills.ReviewSimilarityName

// Status describes the result of an installation request.
type Status string

const (
	StatusInstalled Status = "installed"
	StatusCurrent   Status = "current"
	StatusReplaced  Status = "replaced"
	StatusMissing   Status = "missing"
	StatusDifferent Status = "different"
)

// Result records the installed path and any recoverable backup.
type Result struct {
	Path       string
	BackupPath string
	Status     Status
}

// Inspect compares a project skill with the embedded package without writing.
func Inspect(parent string) (Result, error) {
	if strings.TrimSpace(parent) == "" {
		return Result{}, errors.New("skill parent directory is required")
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return Result{}, fmt.Errorf("resolve skill parent: %w", err)
	}
	destination := filepath.Join(parent, skills.ReviewSimilarityName)
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return Result{Path: destination, Status: StatusMissing}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("inspect installed skill: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Result{}, errors.New("installed skill path must be a real directory, not a symlink")
	}
	files, directories, err := packageContents()
	if err != nil {
		return Result{}, err
	}
	current, err := equalPackage(destination, files, directories)
	if err != nil {
		return Result{}, err
	}
	status := StatusDifferent
	if current {
		status = StatusCurrent
	}
	return Result{Path: destination, Status: status}, nil
}

// Install copies the embedded Mori review skill below parent. Existing
// identical content is accepted. Different content is preserved unless
// replace is explicitly true, in which case it is moved to a sibling backup.
func Install(parent string, replace bool) (Result, error) {
	if strings.TrimSpace(parent) == "" {
		return Result{}, errors.New("skill parent directory is required")
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return Result{}, fmt.Errorf("resolve skill parent: %w", err)
	}
	parent, err = prepareParent(parent)
	if err != nil {
		return Result{}, err
	}

	files, directories, err := packageContents()
	if err != nil {
		return Result{}, err
	}
	destination := filepath.Join(parent, skills.ReviewSimilarityName)
	destinationInfo, err := os.Lstat(destination)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("inspect installed skill: %w", err)
	}
	if exists {
		if destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir() {
			return Result{}, errors.New("installed skill path must be a real directory, not a symlink")
		}
		current, err := equalPackage(destination, files, directories)
		if err != nil {
			return Result{}, err
		}
		if current {
			return Result{Path: destination, Status: StatusCurrent}, nil
		}
		if !replace {
			return Result{}, fmt.Errorf("%w: %s", ErrDifferent, destination)
		}
	}

	temporary, err := os.MkdirTemp(parent, ".mori-skill-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary skill directory: %w", err)
	}
	temporaryCommitted := false
	defer func() {
		if !temporaryCommitted {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := writePackage(temporary, files); err != nil {
		return Result{}, err
	}

	if !exists {
		if _, err := os.Lstat(destination); err == nil {
			return Result{}, errors.New("installed skill path appeared during installation")
		} else if !errors.Is(err, os.ErrNotExist) {
			return Result{}, fmt.Errorf("reinspect installed skill: %w", err)
		}
		if err := os.Rename(temporary, destination); err != nil {
			return Result{}, fmt.Errorf("publish installed skill: %w", err)
		}
		temporaryCommitted = true
		return Result{Path: destination, Status: StatusInstalled}, nil
	}

	backupPlaceholder, err := os.MkdirTemp(
		parent,
		"."+skills.ReviewSimilarityName+".backup-*",
	)
	if err != nil {
		return Result{}, fmt.Errorf("reserve skill backup path: %w", err)
	}
	if err := os.Remove(backupPlaceholder); err != nil {
		return Result{}, fmt.Errorf("prepare skill backup path: %w", err)
	}
	if err := os.Rename(destination, backupPlaceholder); err != nil {
		return Result{}, fmt.Errorf("preserve installed skill: %w", err)
	}
	if err := os.Rename(temporary, destination); err != nil {
		restoreErr := os.Rename(backupPlaceholder, destination)
		if restoreErr != nil {
			return Result{}, fmt.Errorf(
				"publish replacement: %w; restore original: %v",
				err,
				restoreErr,
			)
		}
		return Result{}, fmt.Errorf("publish replacement: %w", err)
	}
	temporaryCommitted = true
	return Result{
		Path:       destination,
		BackupPath: backupPlaceholder,
		Status:     StatusReplaced,
	}, nil
}

func prepareParent(parent string) (string, error) {
	original := parent
	missing := make([]string, 0, 2)
	for {
		info, err := os.Lstat(parent)
		if err == nil {
			if parent == original && info.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("skill parent must be a real directory, not a symlink")
			}
			resolved, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", fmt.Errorf("resolve skill parent links: %w", err)
			}
			resolvedInfo, err := os.Stat(resolved)
			if err != nil {
				return "", fmt.Errorf("inspect resolved skill parent: %w", err)
			}
			if !resolvedInfo.IsDir() {
				return "", errors.New("skill parent must be a directory")
			}
			parent = resolved
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect skill parent: %w", err)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", errors.New("no existing ancestor for skill parent")
		}
		missing = append(missing, filepath.Base(parent))
		parent = next
	}

	for index := len(missing) - 1; index >= 0; index-- {
		parent = filepath.Join(parent, missing[index])
		if err := os.Mkdir(parent, 0o755); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("create skill parent: %w", err)
			}
			info, inspectErr := os.Lstat(parent)
			if inspectErr != nil {
				return "", fmt.Errorf("inspect concurrently created skill parent: %w", inspectErr)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", errors.New("created skill parent must be a real directory")
			}
		}
	}
	return parent, nil
}

func packageContents() (map[string][]byte, map[string]bool, error) {
	packageFS, err := skills.ReviewSimilarity()
	if err != nil {
		return nil, nil, fmt.Errorf("open embedded skill: %w", err)
	}
	files := make(map[string][]byte)
	directories := map[string]bool{".": true}
	if err := fs.WalkDir(packageFS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("embedded skill contains a symlink: %s", path)
		}
		if entry.IsDir() {
			directories[path] = true
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("embedded skill contains a non-regular file: %s", path)
		}
		content, err := fs.ReadFile(packageFS, path)
		if err != nil {
			return fmt.Errorf("read embedded skill file %s: %w", path, err)
		}
		files[path] = content
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return files, directories, nil
}

func equalPackage(
	destination string,
	files map[string][]byte,
	directories map[string]bool,
) (bool, error) {
	seen := make(map[string]bool, len(files))
	equal := true
	err := filepath.WalkDir(destination, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("installed skill contains a symlink: %s", relative)
		}
		if entry.IsDir() {
			if !directories[relative] {
				equal = false
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("installed skill contains a non-regular file: %s", relative)
		}
		expected, ok := files[relative]
		if !ok {
			equal = false
			return nil
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read installed skill file %s: %w", relative, err)
		}
		seen[relative] = true
		if !bytes.Equal(actual, expected) {
			equal = false
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if len(seen) != len(files) {
		equal = false
	}
	return equal, nil
}

func writePackage(destination string, files map[string][]byte) error {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		target := filepath.Join(destination, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create installed skill directory: %w", err)
		}
		if err := os.WriteFile(target, files[path], 0o644); err != nil {
			return fmt.Errorf("write installed skill file %s: %w", path, err)
		}
	}
	return nil
}
