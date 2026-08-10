package source

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/pathutil"
	"github.com/bmatcuk/doublestar/v4"
)

const maxIgnoreBytes = 1024 * 1024

type ignoreRule struct {
	base      string
	pattern   string
	negated   bool
	directory bool
	anchored  bool
}

type ignoreMatcher struct {
	cwd    string
	rules  []ignoreRule
	files  map[string]string
	loaded map[string]struct{}
}

type ignoreLoadError struct {
	err error
}

func (err ignoreLoadError) Error() string {
	return err.err.Error()
}

func (err ignoreLoadError) Unwrap() error {
	return err.err
}

func newIgnoreMatcher(cwd string) *ignoreMatcher {
	return &ignoreMatcher{
		cwd: cwd, files: make(map[string]string), loaded: make(map[string]struct{}),
	}
}

func (matcher *ignoreMatcher) loadDirectory(directory string) error {
	directory = filepath.Clean(directory)
	if _, exists := matcher.loaded[directory]; exists {
		return nil
	}
	matcher.loaded[directory] = struct{}{}
	for _, name := range []string{".gitignore", ".moriignore"} {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", displayPath(matcher.cwd, path), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("ignore file %s is not a regular file", displayPath(matcher.cwd, path))
		}
		if info.Size() > maxIgnoreBytes {
			return fmt.Errorf(
				"ignore file %s exceeds %d bytes",
				displayPath(matcher.cwd, path),
				maxIgnoreBytes,
			)
		}
		digest, err := matcher.loadFile(path, directory, info)
		if err != nil {
			return err
		}
		matcher.files[displayPath(matcher.cwd, path)] = digest
	}
	return nil
}

func (matcher *ignoreMatcher) loadAncestors(boundary string, root string) error {
	boundary = filepath.Clean(boundary)
	root = filepath.Clean(root)
	if !pathutil.Within(boundary, root) {
		return nil
	}
	directories := make([]string, 0)
	for current := filepath.Dir(root); pathutil.Within(boundary, current); current = filepath.Dir(current) {
		directories = append(directories, current)
		if current == boundary {
			break
		}
	}
	for left, right := 0, len(directories)-1; left < right; left, right = left+1, right-1 {
		directories[left], directories[right] = directories[right], directories[left]
	}
	for _, directory := range directories {
		if err := matcher.loadDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func (matcher *ignoreMatcher) loadFile(
	path string,
	base string,
	expectedInfo os.FileInfo,
) (digest string, returnErr error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", displayPath(matcher.cwd, path), err)
	}
	defer func() {
		if err := file.Close(); err != nil && returnErr == nil {
			returnErr = err
		}
	}()

	openedInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("read %s: %w", displayPath(matcher.cwd, path), err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(expectedInfo, openedInfo) {
		return "", fmt.Errorf("ignore file %s changed identity while opening", displayPath(matcher.cwd, path))
	}
	content, err := io.ReadAll(io.LimitReader(file, maxIgnoreBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", displayPath(matcher.cwd, path), err)
	}
	if len(content) > maxIgnoreBytes {
		return "", fmt.Errorf(
			"ignore file %s exceeds %d bytes while reading",
			displayPath(matcher.cwd, path),
			maxIgnoreBytes,
		)
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		rule, ok, err := parseIgnoreRule(base, line)
		if err != nil {
			return "", fmt.Errorf(
				"invalid ignore pattern in %s:%d: %w",
				displayPath(matcher.cwd, path),
				lineNumber,
				err,
			)
		}
		if ok {
			matcher.rules = append(matcher.rules, rule)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", displayPath(matcher.cwd, path), err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(content)), nil
}

func parseIgnoreRule(base string, line string) (ignoreRule, bool, error) {
	line = strings.TrimRight(line, " \t")
	if line == "" {
		return ignoreRule{}, false, nil
	}
	escapedBang := strings.HasPrefix(line, `\!`)
	if strings.HasPrefix(line, `\#`) || escapedBang {
		line = line[1:]
	} else if strings.HasPrefix(line, "#") {
		return ignoreRule{}, false, nil
	}

	negated := !escapedBang && strings.HasPrefix(line, "!")
	if negated {
		line = strings.TrimPrefix(line, "!")
	}
	directory := strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")
	anchored := strings.HasPrefix(line, "/") || strings.Contains(strings.TrimPrefix(line, "/"), "/")
	line = strings.TrimPrefix(line, "/")
	line = filepath.ToSlash(line)
	if line == "" {
		return ignoreRule{}, false, nil
	}
	if _, err := doublestar.Match(line, "probe"); err != nil {
		return ignoreRule{}, false, err
	}
	return ignoreRule{
		base:      filepath.Clean(base),
		pattern:   line,
		negated:   negated,
		directory: directory,
		anchored:  anchored,
	}, true, nil
}

func (matcher *ignoreMatcher) ignored(path string, directory bool) bool {
	ignored := false
	for _, rule := range matcher.rules {
		if !pathutil.Within(rule.base, path) {
			continue
		}
		relative := relativeSlash(rule.base, path)
		if relative == "." || !rule.matches(relative, directory) {
			continue
		}
		ignored = !rule.negated
	}
	return ignored
}

func (rule ignoreRule) matches(relative string, directory bool) bool {
	segments := strings.Split(filepath.ToSlash(relative), "/")
	for index := range segments {
		candidate := strings.Join(segments[:index+1], "/")
		candidateDirectory := index < len(segments)-1 || directory
		if rule.directory && !candidateDirectory {
			continue
		}
		value := candidate
		if !rule.anchored {
			value = segments[index]
		}
		matched, _ := doublestar.Match(rule.pattern, value)
		if matched {
			return true
		}
	}
	return false
}

func (matcher *ignoreMatcher) mayReinclude(path string) bool {
	for _, rule := range matcher.rules {
		if !rule.negated || !pathutil.Within(rule.base, path) {
			continue
		}
		relative := relativeSlash(rule.base, path)
		if relative == "." {
			return true
		}
		// A basename-only negation can re-include entries in directories Mori
		// already traverses, but it is not a reason to reopen every ignored
		// directory below its base. Slash-qualified rules retain Mori's
		// deliberate support for re-including a descendant of an ignored tree.
		if !rule.anchored && !strings.Contains(rule.pattern, "/") {
			continue
		}
		if patternCouldMatchDescendant(rule.pattern, relative) {
			return true
		}
	}
	return false
}

func patternCouldMatchDescendant(pattern string, directory string) bool {
	pattern = filepath.ToSlash(strings.TrimPrefix(pattern, "/"))
	directory = filepath.ToSlash(strings.Trim(directory, "/"))
	if pattern == "" || directory == "" {
		return true
	}
	wildcard := strings.IndexAny(pattern, "*?[")
	if wildcard < 0 {
		return pattern == directory || strings.HasPrefix(pattern, directory+"/")
	}
	prefix := strings.TrimSuffix(pattern[:wildcard], "/")
	if slash := strings.LastIndex(prefix, "/"); slash >= 0 {
		prefix = prefix[:slash]
	} else if wildcard < len(pattern) && !strings.Contains(pattern[:wildcard], "/") {
		prefix = ""
	}
	if prefix == "" {
		return true
	}
	return prefix == directory || strings.HasPrefix(prefix, directory+"/") ||
		strings.HasPrefix(directory, prefix+"/")
}

func (matcher *ignoreMatcher) paths() []string {
	paths := make([]string, 0, len(matcher.files))
	for path := range matcher.files {
		paths = append(paths, path)
	}
	sortStrings(paths)
	return paths
}

func (matcher *ignoreMatcher) evidence() []model.IgnoreFileEvidence {
	paths := matcher.paths()
	evidence := make([]model.IgnoreFileEvidence, 0, len(paths))
	for _, path := range paths {
		evidence = append(evidence, model.IgnoreFileEvidence{Path: path, Digest: matcher.files[path]})
	}
	return evidence
}

func sortStrings(values []string) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && values[cursor] < values[cursor-1]; cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}
