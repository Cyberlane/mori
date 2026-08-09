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
