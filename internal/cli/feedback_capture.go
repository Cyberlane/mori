package cli

import (
	"os"
	"path/filepath"
)

// Avoid aggregating separate project roots under the invoking directory's consent.
func scanFeedbackRoot(options scanOptions, paths []string) string {
	if options.stagedSnapshot != nil {
		return options.stagedSnapshot.Root
	}
	if len(options.changedWorktrees) > 0 || options.includeFocused {
		return ""
	}
	if len(paths) == 0 && len(options.scopeRoots) > 0 {
		paths = options.scopeRoots
	}
	if len(paths) > 1 {
		return ""
	}
	root := "."
	if len(paths) == 1 {
		root = paths[0]
	}
	info, err := os.Stat(root)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		root = filepath.Dir(root)
	}
	return root
}

func feedbackOutcome(code int) string {
	switch code {
	case exitSuccess:
		return "completed"
	case exitFindings:
		return "findings"
	case exitCoverage:
		return "coverage"
	default:
		return "error"
	}
}
