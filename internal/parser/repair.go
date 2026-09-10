package parser

import (
	"bytes"
	"regexp"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

var (
	typescriptImportType    = regexp.MustCompile(`^import\((?:"[A-Za-z0-9_@./:-]+"|'[A-Za-z0-9_@./:-]+')\)(?:\.[A-Za-z_$][A-Za-z0-9_$]*)+$`)
	sqlLimitOffsetParameter = regexp.MustCompile(`(?i)\b(?:LIMIT|OFFSET)[ \t]+(\?|sqlc\.(?:arg|narg)\([^()\r\n]+\))`)
	sqlConflictTarget       = regexp.MustCompile(`(?i)\bON[ \t]+CONFLICT[ \t]*(\([^()\r\n]*\))[ \t]+DO[ \t]+(?:NOTHING|UPDATE)\b`)
	sqlIdentifierList       = regexp.MustCompile(`^\([ \t]*[A-Za-z_][A-Za-z0-9_]*(?:[ \t]*,[ \t]*[A-Za-z_][A-Za-z0-9_]*)*[ \t]*\)$`)
	swiftOptionalTryAwait   = regexp.MustCompile(`\b(try\?[ \t]+await)[ \t]+[A-Za-z_][A-Za-z0-9_]*[ \t]+\{`)
	swiftSwitchAwait        = regexp.MustCompile(`\bswitch([ \t]+)await[ \t]+[A-Za-z_][A-Za-z0-9_]*([ \t]+)\{`)
	swiftEmptyTupleArgument = regexp.MustCompile(`\.[A-Za-z_][A-Za-z0-9_]*\((\(\))\)`)
)

// repairSQLParserInput adapts a small set of valid SQLite and SQLC forms that
// the pinned permissive SQL grammar does not accept. Every replacement keeps
// the original byte length so source locations and source-backed
// normalization remain exact.
func repairSQLParserInput(content []byte) []byte {
	var repaired []byte
	for _, match := range sqlLimitOffsetParameter.FindAllSubmatchIndex(content, -1) {
		start, end := match[2], match[3]
		if start < 0 || end <= start {
			continue
		}
		if repaired == nil {
			repaired = bytes.Clone(content)
		}
		for index := start; index < end; index++ {
			repaired[index] = '1'
		}
	}
	for _, match := range sqlConflictTarget.FindAllSubmatchIndex(content, -1) {
		start, end := match[2], match[3]
		if start < 0 || end <= start || !sqlIdentifierList.Match(content[start:end]) {
			continue
		}
		if repaired == nil {
			repaired = bytes.Clone(content)
		}
		for index := start; index < end; index++ {
			repaired[index] = ' '
		}
	}
	if repaired == nil {
		return content
	}
	return repaired
}

// repairHackParserInput hides an optional first-line shebang from the pinned
// Hack grammar. The grammar accepts legacy <?hh headers but not shebangs; a
// same-length replacement preserves all source offsets and line numbers.
func repairHackParserInput(content []byte) []byte {
	if !bytes.HasPrefix(content, []byte("#!")) {
		return content
	}
	lineEnd := bytes.IndexByte(content, '\n')
	if lineEnd < 0 {
		lineEnd = len(content)
	}
	repaired := bytes.Clone(content)
	for index := 0; index < lineEnd; index++ {
		if repaired[index] != '\r' {
			repaired[index] = ' '
		}
	}
	return repaired
}

// repairSwiftParserInput adapts a bounded set of valid Swift forms that the
// pinned grammar does not accept. Replacements preserve byte length and the
// closest available syntax shape so locations remain exact. The caller only
// accepts a repaired tree when it reduces parser diagnostics.
func repairSwiftParserInput(content []byte) []byte {
	var repaired []byte
	clone := func() []byte {
		if repaired == nil {
			repaired = bytes.Clone(content)
		}
		return repaired
	}

	for _, match := range swiftOptionalTryAwait.FindAllSubmatchIndex(content, -1) {
		start, end := match[2], match[3]
		if start >= 0 && end > start {
			for index := start; index < end; index++ {
				clone()[index] = ' '
			}
		}
	}
	for _, match := range swiftSwitchAwait.FindAllSubmatchIndex(content, -1) {
		leadingStart, trailingEnd := match[2], match[5]
		if leadingStart >= 0 && trailingEnd > 0 {
			clone()[leadingStart] = '('
			clone()[trailingEnd-1] = ')'
		}
	}
	for _, match := range swiftEmptyTupleArgument.FindAllSubmatchIndex(content, -1) {
		start, end := match[2], match[3]
		if start >= 0 && end-start == 2 {
			clone()[start] = '['
			clone()[start+1] = ']'
		}
	}
	return repaired
}

// repairJSXParserInput works around the pinned TypeScript/JavaScript grammar's
// raw-ampersand JSX text bug. It repairs only error nodes directly contained
// by JSX elements and leaves entities and expression errors untouched.
func repairJSXParserInput(root *tree_sitter.Node, content []byte) []byte {
	if root == nil || !root.HasError() {
		return nil
	}
	var repaired []byte
	cursor := root.Walk()
	defer cursor.Close()

	for {
		current := cursor.Node()
		if current.IsError() && isJSXTextError(current) {
			start := int(current.StartByte())
			end := int(current.EndByte())
			if start >= 0 && end <= len(content) && start < end {
				for offset := start; offset < end; offset++ {
					if content[offset] != '&' || isJSXEntity(content[offset:end]) {
						continue
					}
					if repaired == nil {
						repaired = bytes.Clone(content)
					}
					repaired[offset] = ' '
				}
			}
		}

		if cursor.GotoFirstChild() {
			continue
		}
		for {
			if cursor.GotoNextSibling() {
				break
			}
			if !cursor.GotoParent() {
				return repaired
			}
		}
	}
}

func isJSXTextError(node *tree_sitter.Node) bool {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "jsx_expression":
			return false
		case "jsx_element":
			return true
		}
	}
	return false
}

func isJSXEntity(content []byte) bool {
	if len(content) < 3 || content[0] != '&' {
		return false
	}
	value := string(content)
	semicolon := strings.IndexByte(value, ';')
	if semicolon < 2 {
		return false
	}
	entity := value[1:semicolon]
	if entity[0] == '#' {
		entity = entity[1:]
		if entity == "" {
			return false
		}
		if entity[0] == 'x' || entity[0] == 'X' {
			entity = entity[1:]
			if entity == "" {
				return false
			}
			for _, character := range entity {
				if !((character >= '0' && character <= '9') ||
					(character >= 'a' && character <= 'f') ||
					(character >= 'A' && character <= 'F')) {
					return false
				}
			}
			return true
		}
		for _, character := range entity {
			if character < '0' || character > '9' {
				return false
			}
		}
		return true
	}
	for _, character := range entity {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9')) {
			return false
		}
	}
	return true
}

// repairTypeScriptImportTypes adapts import("module").Type references that the
// pinned grammar misreads as expressions inside ambient interface properties.
// Only complete import type error nodes are eligible; runtime imports and
// malformed type expressions retain their diagnostics. No function annotation
// is rewritten. The placeholder preserves byte offsets.
func repairTypeScriptImportTypes(root *tree_sitter.Node, content []byte) []byte {
	if root == nil || !root.HasError() {
		return nil
	}
	var repaired []byte
	cursor := root.Walk()
	defer cursor.Close()
	for {
		current := cursor.Node()
		parent := current.Parent()
		if current.IsError() && parent != nil && parent.Kind() == "type_annotation" && isAmbientInterfaceProperty(parent) {
			start, end := int(current.StartByte()), int(current.EndByte())
			if start >= 0 && end <= len(content) && start < end && typescriptImportType.Match(content[start:end]) {
				// Only the import qualifier becomes a placeholder type identifier. Keep
				// the qualified member and array syntax for the original type shape.
				close := bytes.IndexByte(content[start:end], ')')
				if repaired == nil {
					repaired = bytes.Clone(content)
				}
				repaired[start] = 'T'
				for offset := start + 1; offset <= start+close; offset++ {
					repaired[offset] = ' '
				}
			}
		}
		if cursor.GotoFirstChild() {
			continue
		}
		for {
			if cursor.GotoNextSibling() {
				break
			}
			if !cursor.GotoParent() {
				return repaired
			}
		}
	}
}

func isAmbientInterfaceProperty(annotation *tree_sitter.Node) bool {
	parent := annotation.Parent()
	if parent == nil || parent.Kind() != "property_signature" {
		return false
	}
	parent = parent.Parent()
	if parent == nil || parent.Kind() != "interface_body" {
		return false
	}
	for parent = parent.Parent(); parent != nil; parent = parent.Parent() {
		if parent.Kind() == "ambient_declaration" {
			return true
		}
	}
	return false
}
