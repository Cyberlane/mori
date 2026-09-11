package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
)

// This is a presentation heuristic, never an intentional-duplication judgment.
// Require repeated same-name small call wrappers in four distinct directories.
// Arithmetic, branches, loops, assignments and other explicit flow keep their
// normal priority even when short and repeated. No function name is blacklisted.
func repeatedSmallWrapper(candidate *groupCandidate) bool {
	if !smallCallWrapper(candidate.left) || !smallCallWrapper(candidate.right) {
		return false
	}
	name := ""
	crossDirectory := false
	dirs := make(map[string]struct{})
	for _, pair := range candidate.pathPairs {
		crossDirectory = crossDirectory || filepath.Dir(pair.Left.Path) != filepath.Dir(pair.Right.Path)
		for _, location := range []model.Location{pair.Left, pair.Right} {
			if !distinctiveReviewName(location.Name) {
				return false
			}
			if name == "" {
				name = location.Name
			}
			if location.Name != name {
				return false
			}
			dirs[filepath.Dir(location.Path)] = struct{}{}
		}
	}
	return crossDirectory && len(dirs) >= 4
}

func smallCallWrapper(fragment model.Fragment) bool {
	if fragment.TokenCount <= 0 || fragment.TokenCount > 80 || fragment.NestedCount > 0 || fragment.Features["node:expression:call"] == 0 {
		return false
	}
	for feature, count := range fragment.Features {
		if count == 0 {
			continue
		}
		if strings.HasPrefix(feature, "node:flow:") && feature != "node:flow:return" {
			return false
		}
		switch feature {
		case "node:expression:binary", "node:expression:unary", "node:expression:assignment", "node:binding":
			return false
		}
	}
	return true
}
