package cli

import (
	"fmt"
	"github.com/Cyberlane/mori/internal/parser"
	"path/filepath"
	"strings"
)

func configSelectionPaths(base string, paths []string) []string {
	result := make([]string, len(paths))
	for i, path := range paths {
		if strings.TrimSpace(path) == "" {
			result[i] = ""
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}
		result[i] = displayCLIPath(path)
	}
	return result
}

func resolvedSelectionPaths(options scanOptions) ([]string, []string, error) {
	resolve := func(paths []string) ([]string, error) {
		if len(paths) > 256 {
			return nil, fmt.Errorf("at most 256 classification paths are allowed")
		}
		result := make([]string, len(paths))
		for i, path := range paths {
			if strings.TrimSpace(path) == "" || len(path) > 4096 || strings.ContainsAny(path, "\x00\r\n") {
				return nil, fmt.Errorf("classification paths must be nonempty bounded paths")
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			result[i] = absolute
		}
		return result, nil
	}
	production, err := resolve(options.productionPaths)
	if err != nil {
		return nil, nil, err
	}
	tests, err := resolve(options.testPaths)
	if err != nil {
		return nil, nil, err
	}
	if err := parser.ValidateSelectionPaths(production, tests); err != nil {
		return nil, nil, err
	}
	return production, tests, nil
}

// Policy labels use the same canonical location as selection while remaining
// portable within the working root. Sorting is handled by baseline.Digest.
func selectionPolicyLabels(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	labels := make([]string, len(paths))
	for i, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			labels[i] = path
			continue
		}
		labels[i] = displayCLIPath(parser.CanonicalSelectionPath(absolute))
	}
	return labels
}
