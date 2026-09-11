package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/pathutil"
)

// The hint describes a bounded sample of retained locations, not precision or
// repository-wide composition. It never changes scan policy or machine JSON.
func scopeHint(w io.Writer, value model.Report) error {
	sampled := min(25, len(value.Groups))
	if sampled < 5 {
		return nil
	}
	testGroups := 0
	for _, group := range value.Groups[:sampled] {
		found := false
		for _, profile := range group.Profiles {
			for _, occurrence := range profile.Occurrences {
				if conventionalReviewTestPath(occurrence.Location.Path) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			testGroups++
		}
	}
	if testGroups*2 < sampled {
		return nil
	}
	_, err := fmt.Fprintf(w, "scope hint: %d/%d leading retained groups contain test/story paths (path conventions, not false-positive counts). For production review, run mori setup from the project root and choose a library or application scope; then use its printed scan command. Existing projects: use mori configure.\n", testGroups, sampled)
	return err
}

func conventionalReviewTestPath(path string) bool {
	path = pathutil.PortableSlash(path)
	parts := strings.Split(path, "/")
	// Absolute and parent-relative paths may include unrelated ancestor names.
	if !pathutil.IsRooted(path) && parts[0] != ".." {
		for _, part := range parts[:len(parts)-1] {
			switch part {
			case "test", "tests", "__tests__", "spec", "specs", "stories", "__stories__":
				return true
			}
		}
	}
	base := parts[len(parts)-1]
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.Contains(base, ".stories.") || strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") || base == "test.js"
}
