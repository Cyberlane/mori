// Package source discovers supported source files without following symlinks.
package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/diagnostic"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/pathutil"
	"github.com/bmatcuk/doublestar/v4"
)

// File is a source file paired with its parser grammar.
type File struct {
	Path           string
	DisplayPath    string
	Language       language.Spec
	AnalysisDomain string
	Generated      bool
	Marker         string
	MaxBytes       int64
	Info           os.FileInfo
}

// Options controls file discovery.
type Options struct {
	Excludes          []string
	MaxFileBytes      int64
	IgnoreFiles       bool
	ComparisonDomains map[string]struct{}
	SQLDialect        string
	EmbeddedSQL       bool
	ExcludeGenerated  bool
}

// Result contains deterministic discovery output and recoverable warnings.
type Result struct {
	Files       []File
	Warnings    []model.Warning
	IgnoreFiles []string
	Excluded    []model.FileCoverage
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
		Excluded: make([]model.FileCoverage, 0),
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
			if pathutil.Within(cwd, root) {
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
	sort.Slice(result.Excluded, func(i, j int) bool {
		return result.Excluded[i].Path < result.Excluded[j].Path
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
	spec, supported := language.DetectWithSQLDialect(path, options.SQLDialect)
	needsShebang := !supported && filepath.Ext(path) == ""
	if !supported && !needsShebang {
		if explicit {
			result.Warnings = append(result.Warnings, model.Warning{
				Path:    displayPath(cwd, path),
				Message: "unsupported source extension",
			})
		}
		return
	}
	if supported && len(options.ComparisonDomains) > 0 {
		if _, selected := options.ComparisonDomains[spec.ComparisonDomain]; !selected {
			_, sqlSelected := options.ComparisonDomains["sql-query"]
			if !options.EmbeddedSQL || !sqlSelected || spec.ID != "go" {
				return
			}
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
	if needsShebang {
		firstLine, err := readShebangLine(path, info)
		if err != nil {
			result.Warnings = append(result.Warnings, warning(displayPath(cwd, path), err))
			return
		}
		spec, supported = language.DetectShebang(firstLine)
		if !supported {
			if explicit {
				result.Warnings = append(result.Warnings, model.Warning{
					Path:    displayPath(cwd, path),
					Message: "unsupported source extension or shebang",
				})
			}
			return
		}
		if len(options.ComparisonDomains) > 0 {
			if _, selected := options.ComparisonDomains[spec.ComparisonDomain]; !selected {
				return
			}
		}
	}
	analysisDomain := spec.ComparisonDomain
	if options.EmbeddedSQL && spec.ID == "go" {
		if _, selected := options.ComparisonDomains["sql-query"]; selected {
			analysisDomain = "sql-query"
		}
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
	generated, marker, err := detectGeneratedHeader(path, info)
	if err != nil {
		result.Warnings = append(result.Warnings, warning(displayPath(cwd, path), err))
		return
	}

	cleanPath := filepath.Clean(path)
	if _, exists := seen[cleanPath]; exists {
		return
	}
	seen[cleanPath] = struct{}{}
	if generated && options.ExcludeGenerated {
		result.Excluded = append(result.Excluded, model.FileCoverage{
			Path:             displayPath(cwd, cleanPath),
			Language:         spec.ID,
			LanguageFamily:   spec.Family,
			ComparisonDomain: analysisDomain,
			Status:           "excluded_generated",
			Generated:        true,
			GeneratedMarker:  marker,
		})
		return
	}
	result.Files = append(result.Files, File{
		Path:           cleanPath,
		DisplayPath:    displayPath(cwd, cleanPath),
		Language:       spec,
		AnalysisDomain: analysisDomain,
		Generated:      generated,
		Marker:         marker,
		MaxBytes:       options.MaxFileBytes,
		Info:           info,
	})
}

const maxGeneratedHeaderBytes = 8192

func detectGeneratedHeader(path string, expected os.FileInfo) (bool, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, "", err
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		return false, "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(expected, opened) {
		return false, "", errors.New("source changed during generated-file inspection")
	}

	content, err := io.ReadAll(io.LimitReader(file, maxGeneratedHeaderBytes))
	if err != nil {
		return false, "", err
	}
	inBlockComment := false
	for _, line := range strings.Split(string(content), "\n") {
		comment, ok := generatedComment(line, &inBlockComment)
		if !ok {
			continue
		}
		lower := strings.ToLower(comment)
		switch {
		case strings.HasPrefix(lower, "generated by typeshare"):
			return true, "typeshare-generated", nil
		case strings.Contains(lower, "code generated") && strings.Contains(lower, "do not edit"):
			return true, "code-generated-do-not-edit", nil
		case strings.Contains(lower, "this file was automatically generated"):
			return true, "automatically-generated", nil
		case strings.Contains(lower, "this file is generated") && strings.Contains(lower, "do not edit"):
			return true, "generated-do-not-edit", nil
		case strings.Contains(lower, "auto-generated") && strings.Contains(lower, "do not edit"):
			return true, "auto-generated-do-not-edit", nil
		case strings.Contains(lower, "@generated"):
			return true, "at-generated", nil
		}
	}
	return false, "", nil
}

func generatedComment(line string, inBlock *bool) (string, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if *inBlock {
		if end := strings.Index(trimmed, "*/"); end >= 0 {
			trimmed = trimmed[:end]
			*inBlock = false
		}
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "*")), true
	}
	if strings.HasPrefix(trimmed, "/*") {
		comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "/*"))
		if end := strings.Index(comment, "*/"); end >= 0 {
			comment = comment[:end]
		} else {
			*inBlock = true
		}
		return strings.TrimSpace(comment), true
	}
	for _, prefix := range []string{"//", "#", "--", "<!--"} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
		}
	}
	return "", false
}

const maxShebangBytes = 256

func readShebangLine(path string, expected os.FileInfo) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(expected, opened) {
		return "", errors.New("source changed during discovery")
	}

	content, err := io.ReadAll(io.LimitReader(file, maxShebangBytes+1))
	if err != nil {
		return "", err
	}
	lineEnd := len(content)
	if index := strings.IndexByte(string(content), '\n'); index >= 0 {
		lineEnd = index
	}
	if lineEnd > maxShebangBytes {
		return "", nil
	}
	return strings.TrimSuffix(string(content[:lineEnd]), "\r"), nil
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
		if parent == current || !pathutil.Within(boundary, parent) {
			return false, nil
		}
		current = parent
	}
}

func trustedBoundary(cwd string, path string) string {
	volumeRoot := filepath.VolumeName(path) + string(filepath.Separator)
	best := filepath.Clean(volumeRoot)
	for _, candidate := range []string{cwd, os.TempDir()} {
		absoluteCandidate, err := filepath.Abs(candidate)
		if err != nil || !pathutil.Within(absoluteCandidate, path) {
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
	if err == nil && pathutil.Within(cwd, path) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}

func warning(path string, err error) model.Warning {
	return model.Warning{Path: path, Message: diagnostic.Message(err)}
}
