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

// repeatedSmallBoilerplate is deliberately conservative: only repeated short
// straight-line bodies qualify. A classification affects presentation, never
// whether a match exists or is actionable. Names are not a blacklist.
func repeatedSmallBoilerplate(candidate *groupCandidate) bool {
	if !smallStraightLineBody(candidate.left) || !smallStraightLineBody(candidate.right) {
		return false
	}
	files := make(map[string]struct{})
	locations := make(map[model.Location]struct{})
	for _, pair := range candidate.pathPairs {
		for _, location := range []model.Location{pair.Left, pair.Right} {
			files[location.Path] = struct{}{}
			locations[location] = struct{}{}
		}
		if len(files) >= 2 && len(locations) >= 3 {
			return true
		}
	}
	return false
}

func smallStraightLineBody(fragment model.Fragment) bool {
	if fragment.TokenCount <= 0 || fragment.TokenCount > 80 || fragment.NestedCount > 0 {
		return false
	}
	operations := 0
	for feature, count := range fragment.Features {
		if count == 0 || !strings.HasPrefix(feature, "node:") {
			continue
		}
		if strings.HasPrefix(feature, "node:operator:") && feature != "node:operator:assign" {
			return false
		}
		if strings.HasPrefix(feature, "node:flow:") {
			if feature != "node:flow:return" && feature != "node:flow:throw" {
				return false
			}
			operations += count
		}
		if strings.HasPrefix(feature, "node:expression:") {
			switch feature {
			case "node:expression:call", "node:expression:member", "node:expression:assignment", "node:expression:new", "node:expression:subscript", "node:expression:parenthesized":
			default:
				return false
			}
		}
		switch feature {
		case "node:binding", "node:function:nested":
			return false
		case "node:expression:call", "node:expression:assignment", "node:expression:new", "node:statement:throw":
			operations += count
		}
	}
	// One accessor/setter, one call, or return/throw wrapping one call.
	return operations > 0 && operations <= 2 && fragment.Features["node:expression:assignment"] <= 1
}
