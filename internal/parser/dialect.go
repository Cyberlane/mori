package parser

import (
	ts "github.com/tree-sitter/go-tree-sitter"
	"strings"
)

// A pragma is evidence of a dialect, not permission to erase its syntax. Only
// leading comments count, so string literals and comments in bodies cannot
// relabel ordinary JavaScript errors as an unsupported dialect.
func hasFlowPragma(root *ts.Node, content []byte) bool {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Kind() == "hash_bang_line" {
			continue
		}
		if child.Kind() != "comment" {
			return false
		}
		text := strings.TrimPrefix(strings.TrimPrefix(child.Utf8Text(content), "//"), "/*")
		text = strings.TrimSuffix(text, "*/")
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == "@flow" {
				return true
			}
		}
	}
	return false
}
