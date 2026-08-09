// Package source discovers supported source files without following symlinks.
package source

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/diagnostic"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/bmatcuk/doublestar/v4"
)

// File is a source file paired with its parser grammar.
type File struct {
	Path        string
	DisplayPath string
	Language    language.Spec
	MaxBytes    int64
	Info        os.FileInfo
}

// Options controls file discovery.
type Options struct {
	Excludes          []string
	MaxFileBytes      int64
	IgnoreFiles       bool
	ComparisonDomains map[string]struct{}
}

// Result contains deterministic discovery output and recoverable warnings.
type Result struct {
	Files       []File
	Warnings    []model.Warning
	IgnoreFiles []string
}

var defaultExcludedDirectories = map[string]struct{}{
	".git":         {},
	".hg":          {},
	".svn":         {},
	".turbo":       {},
	"build":        {},
	"coverage":     {},
	"dist":         {},
	"node_modules": {},
	"target":       {},
	"vendor":       {},
}

// Discover resolves files below paths. Unsupported files are ignored inside
// directories and reported when explicitly requested.
func Discover(paths []string, options Options) Result {
	result, _ := DiscoverContext(context.Background(), paths, options)
	return result
}

// DiscoverContext resolves files below paths and stops when the context is
// canceled.
func DiscoverContext(ctx context.Context, paths []string, options Options) (Result, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	result := Result{
		Files:    make([]File, 0),
		Warnings: make([]model.Warning, 0),
	}
	seen := make(map[string]struct{})
	ignores := newIgnoreMatcher(cwd)

	for _, requestedPath := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		absolutePath, err := filepath.Abs(requestedPath)
		if err != nil {
			result.Warnings = append(result.Warnings, warning(requestedPath, err))
			continue
		}

		info, err := os.Lstat(absolutePath)
		if err != nil {
			result.Warnings = append(result.Warnings, warning(requestedPath, err))
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			result.Warnings = append(result.Warnings, model.Warning{
				Path:    displayPath(cwd, absolutePath),
				Message: "symbolic links are not followed",
			})
			continue
		}
		hasSymlink, err := hasSymlinkComponent(
			absolutePath,
			trustedBoundary(cwd, absolutePath),
		)
		if err != nil {
			result.Warnings = append(result.Warnings, warning(requestedPath, err))
			continue
		}
		if hasSymlink {
			result.Warnings = append(result.Warnings, model.Warning{
				Path:    displayPath(cwd, absolutePath),
				Message: "paths containing symbolic links are not followed",
			})
			continue
		}

		if !info.IsDir() {
			if matchesAny(
				filepath.Base(absolutePath),
				displayPath(cwd, absolutePath),
				options.Excludes,
			) {
				continue
			}
			addFile(&result, seen, cwd, absolutePath, true, options)
			continue
		}

		root := absolutePath
		if options.IgnoreFiles {
			boundary := root
			if pathWithin(cwd, root) {
				boundary = cwd
			}
			if err := ignores.loadAncestors(boundary, root); err != nil {
				return result, err
			}
		}
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				result.Warnings = append(result.Warnings, warning(displayPath(cwd, path), walkErr))
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			if entry.IsDir() && options.IgnoreFiles {
				if err := ignores.loadDirectory(path); err != nil {
					return ignoreLoadError{err: err}
				}
			}

			if path != root && entry.IsDir() {
				if _, excluded := defaultExcludedDirectories[strings.ToLower(entry.Name())]; excluded {
					return fs.SkipDir
				}
				if matchesAny(relativeSlash(root, path), displayPath(cwd, path), options.Excludes) {
					return fs.SkipDir
				}
				if options.IgnoreFiles && ignores.ignored(path, true) {
					if ignores.mayReinclude(path) {
						return nil
					}
					return fs.SkipDir
				}
				return nil
			}

			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
				return nil
			}
			if matchesAny(relativeSlash(root, path), displayPath(cwd, path), options.Excludes) {
				return nil
			}
			if options.IgnoreFiles && ignores.ignored(path, false) {
				return nil
			}
			addFile(&result, seen, cwd, path, false, options)
			return nil
		})
		if walkErr != nil {
			if errors.Is(walkErr, context.Canceled) ||
				errors.Is(walkErr, context.DeadlineExceeded) {
				return result, walkErr
			}
			var ignoreErr ignoreLoadError
			if errors.As(walkErr, &ignoreErr) {
				return result, ignoreErr.err
			}
			result.Warnings = append(result.Warnings, warning(displayPath(cwd, root), walkErr))
		}
	}
	result.IgnoreFiles = ignores.paths()

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].DisplayPath < result.Files[j].DisplayPath
	})
	sort.Slice(result.Warnings, func(i, j int) bool {
		if result.Warnings[i].Path == result.Warnings[j].Path {
			return result.Warnings[i].Message < result.Warnings[j].Message
		}
		return result.Warnings[i].Path < result.Warnings[j].Path
	})
	return result, nil
}

// ValidatePatterns checks exclude patterns before a scan starts.
func ValidatePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if _, err := doublestar.Match(pattern, "probe"); err != nil {
			return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
	}
	return nil
}

func addFile(
	result *Result,
	seen map[string]struct{},
	cwd string,
	path string,
	explicit bool,
	options Options,
) {
	spec, supported := language.Detect(path)
	if !supported {
		if explicit {
			result.Warnings = append(result.Warnings, model.Warning{
				Path:    displayPath(cwd, path),
				Message: "unsupported source extension",
			})
		}
		return
	}
	if len(options.ComparisonDomains) > 0 {
		if _, selected := options.ComparisonDomains[spec.ComparisonDomain]; !selected {
			return
		}
	}

	info, err := os.Lstat(path)
	if err != nil {
		result.Warnings = append(result.Warnings, warning(displayPath(cwd, path), err))
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		result.Warnings = append(result.Warnings, model.Warning{
			Path:    displayPath(cwd, path),
			Message: "symbolic links are not followed",
		})
		return
	}
	if !info.Mode().IsRegular() {
		if explicit {
			result.Warnings = append(result.Warnings, model.Warning{
				Path:    displayPath(cwd, path),
				Message: "not a regular file",
			})
		}
		return
	}
	if options.MaxFileBytes > 0 && info.Size() > options.MaxFileBytes {
		result.Warnings = append(result.Warnings, model.Warning{
			Path: displayPath(cwd, path),
			Message: fmt.Sprintf(
				"file is %d bytes; limit is %d bytes",
				info.Size(),
				options.MaxFileBytes,
			),
		})
		return
	}

	cleanPath := filepath.Clean(path)
	if _, exists := seen[cleanPath]; exists {
		return
	}
	seen[cleanPath] = struct{}{}
	result.Files = append(result.Files, File{
		Path:        cleanPath,
		DisplayPath: displayPath(cwd, cleanPath),
		Language:    spec,
		MaxBytes:    options.MaxFileBytes,
		Info:        info,
	})
}

func hasSymlinkComponent(path string, boundary string) (bool, error) {
	current := filepath.Clean(path)
	boundary = filepath.Clean(boundary)
	for {
		if current == boundary {
			return false, nil
		}
		info, err := os.Lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
		parent := filepath.Dir(current)
		if parent == current || !pathWithin(boundary, parent) {
			return false, nil
		}
		current = parent
	}
}

func pathWithin(base string, path string) bool {
	relative, err := filepath.Rel(base, path)
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func trustedBoundary(cwd string, path string) string {
	volumeRoot := filepath.VolumeName(path) + string(filepath.Separator)
	best := filepath.Clean(volumeRoot)
	for _, candidate := range []string{cwd, os.TempDir()} {
		absoluteCandidate, err := filepath.Abs(candidate)
		if err != nil || !pathWithin(absoluteCandidate, path) {
			continue
		}
		if len(absoluteCandidate) > len(best) {
			best = filepath.Clean(absoluteCandidate)
		}
	}
	return best
}

func matchesAny(rootRelativePath string, cwdRelativePath string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		rootMatch, _ := doublestar.Match(pattern, rootRelativePath)
		cwdMatch, _ := doublestar.Match(pattern, filepath.ToSlash(cwdRelativePath))
		baseMatch, _ := doublestar.Match(pattern, filepath.Base(rootRelativePath))
		if rootMatch || cwdMatch || baseMatch {
			return true
		}
	}
	return false
}

func relativeSlash(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func displayPath(cwd string, path string) string {
	relative, err := filepath.Rel(cwd, path)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}

func warning(path string, err error) model.Warning {
	return model.Warning{Path: path, Message: diagnostic.Message(err)}
}
