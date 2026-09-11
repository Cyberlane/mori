package parser

import (
	"strings"
	"unicode"

	"github.com/Cyberlane/mori/internal/pathutil"
	"github.com/Cyberlane/mori/internal/source"
	ts "github.com/tree-sitter/go-tree-sitter"
)

// isTestFragment uses explicit conventional paths or positive Rust attributes.
// Unrecognized constructs remain in production selection; this is not a claim
// that Mori can infer a function's purpose from its behavior or name.
func isTestFragment(node *ts.Node, content []byte, file source.File) bool {
	classification := file.ClassificationPath
	if classification == "" {
		classification = file.DisplayPath
	}
	path := pathutil.PortableSlash(classification)
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		if pathutil.IsRooted(classification) || parts[0] == ".." {
			break
		}
		if part == "test" || part == "tests" || part == "__tests__" {
			return true
		}
	}
	base := parts[len(parts)-1]
	for _, suffix := range []string{"_test.go", "_test.rs", "_test.py", ".test.js", ".test.jsx", ".test.ts", ".test.tsx", ".spec.js", ".spec.jsx", ".spec.ts", ".spec.tsx"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return true
	}
	if file.Language.ID != "rust" {
		return false
	}
	for current := node; current != nil; current = current.Parent() {
		for attr := current.PrevNamedSibling(); attr != nil; attr = attr.PrevNamedSibling() {
			if attr.Kind() == "line_comment" || attr.Kind() == "block_comment" {
				continue
			}
			if attr.Kind() != "attribute_item" {
				break
			}
			text := string(content[attr.StartByte():attr.EndByte()])
			text = strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) {
					return -1
				}
				return r
			}, text)
			if text == "#[test]" || strings.HasPrefix(text, "#[tokio::test(") || text == "#[tokio::test]" || strings.HasPrefix(text, "#[async_std::test(") || text == "#[async_std::test]" {
				return true
			}
			if strings.HasPrefix(text, "#[cfg(") && strings.HasSuffix(text, ")]") {
				predicate := text[len("#[cfg(") : len(text)-2]
				if cfgRequiresTest(predicate) {
					return true
				}
			}
		}
	}
	return false
}

// cfgRequiresTest proves implication, rather than merely searching for the word
// test: all(test, ...) requires test; any(test, feature="prod") does not.
func cfgRequiresTest(predicate string) bool {
	if len(predicate) > 8192 {
		return false
	}
	return cfgRequiresTestAtDepth(predicate, 0)
}

func cfgRequiresTestAtDepth(predicate string, depthLimit int) bool {
	if depthLimit > 64 {
		return false
	}
	if predicate == "test" {
		return true
	}
	all := strings.HasPrefix(predicate, "all(")
	any := strings.HasPrefix(predicate, "any(")
	if (!all && !any) || !strings.HasSuffix(predicate, ")") {
		return false
	}
	inner := predicate[4 : len(predicate)-1]
	args := []string{}
	start, depth := 0, 0
	quoted, escaped := false, false
	for i, c := range inner {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, inner[start:i])
				start = i + 1
			}
		}
	}
	args = append(args, inner[start:])
	found := false
	for _, arg := range args {
		if arg == "" {
			continue
		}
		requires := cfgRequiresTestAtDepth(arg, depthLimit+1)
		if all && requires {
			return true
		}
		if any && !requires {
			return false
		}
		found = true
	}
	return any && found
}

func excludeSelectedFragment(node *ts.Node, content []byte, file source.File, selection string, coverage *Coverage) bool {
	if selection == "" || selection == "all" {
		return false
	}
	test := isTestFragment(node, content, file)
	if selection == "production" && test {
		coverage.ExcludedTestFragments++
		return true
	}
	if selection == "tests" && !test {
		coverage.ExcludedProductionFragments++
		return true
	}
	return false
}
