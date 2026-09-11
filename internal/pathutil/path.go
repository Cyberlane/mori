// Package pathutil provides shared lexical path checks.
package pathutil

import (
	"path/filepath"
	"strings"
)

// Within reports whether path is base or a lexical descendant of base.
// It does not resolve symbolic links.
func Within(base string, path string) bool {
	relative, err := filepath.Rel(base, path)
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// IsRooted recognizes POSIX roots and Windows roots/drive-qualified paths on
// every host. Use it when classifying portable report paths, where filepath.IsAbs
// alone would treat a foreign platform's root as a project-relative directory.
// Drive-relative Windows paths also carry an external anchor and are excluded
// from project-relative ancestry heuristics.
func IsRooted(path string) bool {
	if path == "" {
		return false
	}
	if path[0] == '/' || path[0] == '\\' {
		return true
	}
	return len(path) >= 2 && path[1] == ':' && (path[0] >= 'a' && path[0] <= 'z' || path[0] >= 'A' && path[0] <= 'Z')
}

// PortableSlash normalizes separators for lexical classification of report paths
// from either platform. It must not be used to resolve actual filesystem paths.
func PortableSlash(path string) string { return strings.ReplaceAll(path, `\`, "/") }
