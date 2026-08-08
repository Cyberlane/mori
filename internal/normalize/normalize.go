// Package normalize converts grammar-specific trees into shared feature bags.
package normalize

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Cyberlane/mori/internal/model"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Version identifies the normalization contract used to build feature bags.
// Bump it whenever the feature vocabulary, weights, canonical mappings, or
// semantic-hint list changes.
const Version = 1

// Profile is a normalized, language-neutral view of one syntax fragment.
// It is not the stable content identity exposed in reports.
type Profile struct {
	Features   model.FeatureBag
	TokenCount int
}

// Build creates a fingerprint and excludes nested function bodies. Nested
// functions are analyzed as independent fragments by the parser.
func Build(
	ctx context.Context,
	root *tree_sitter.Node,
	source []byte,
	isBoundary func(string) bool,
	excludeNestedBoundaries bool,
) (Profile, error) {
	profile := Profile{Features: make(model.FeatureBag)}
	if root == nil {
		return profile, nil
	}

	type frame struct {
		node            *tree_sitter.Node
		parentCanonical string
		field           string
		childParent     string
		nextChild       uint
		entered         bool
	}
	stack := []frame{{node: root}}
	rootID := root.Id()

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return Profile{}, err
		}

		current := &stack[len(stack)-1]
		if !current.entered {
			childParent, descend := enterNode(
				current.node,
				rootID,
				current.parentCanonical,
				current.field,
				source,
				isBoundary,
				excludeNestedBoundaries,
				&profile,
			)
			if !descend {
				stack = stack[:len(stack)-1]
				continue
			}
			current.childParent = childParent
			current.entered = true
		}

		if current.nextChild >= current.node.ChildCount() {
			stack = stack[:len(stack)-1]
			continue
		}
		childIndex := current.nextChild
		current.nextChild++
		child := current.node.Child(childIndex)
		if child == nil {
			continue
		}
		stack = append(stack, frame{
			node:            child,
			parentCanonical: current.childParent,
			field:           current.node.FieldNameForChild(uint32(childIndex)),
		})
	}

	return profile, nil
}

func enterNode(
	node *tree_sitter.Node,
	rootID uintptr,
	parentCanonical string,
	field string,
	source []byte,
	isBoundary func(string) bool,
	excludeNestedBoundaries bool,
	profile *Profile,
) (string, bool) {
	if node == nil || node.IsError() || node.IsMissing() || node.IsExtra() {
		return parentCanonical, false
	}

	kind := node.Kind()
	if excludeNestedBoundaries && node.Id() != rootID && isBoundary(kind) {
		addFeature(profile.Features, "node:function:nested", 1)
		if parentCanonical != "" {
			addFeature(profile.Features, "edge:"+parentCanonical+">function:nested", 1)
		}
		profile.TokenCount++
		return parentCanonical, false
	}

	if shouldSkipSubtree(kind) {
		return parentCanonical, false
	}

	canonical := ""
	if node.IsNamed() {
		canonical = canonicalNamed(kind)
	} else {
		canonical = canonicalOperator(kind)
	}

	nextParent := parentCanonical
	if canonical != "" {
		addFeature(profile.Features, "node:"+canonical, 1)
		if class := coarseClass(canonical); class != "" {
			addFeature(profile.Features, "class:"+class, 1)
		}
		profile.TokenCount++
		if parentCanonical != "" {
			addFeature(profile.Features, "edge:"+parentCanonical+">"+canonical, 1)
		}
		if role := canonicalRole(field); role != "" {
			addFeature(profile.Features, "role:"+role+">"+canonical, 1)
		}
		nextParent = canonical
	}

	if operation := semanticOperation(node, source); operation != "" {
		// Semantic operation families are deliberate hints, not claims of
		// behavioral equivalence. A weight of two keeps them useful without
		// overpowering the surrounding tree structure.
		addFeature(profile.Features, "semantic:"+operation, 2)
	}
	if !node.IsNamed() && canonical == "operator:membership" {
		addFeature(profile.Features, "semantic:membership", 2)
	}

	return nextParent, true
}

func canonicalNamed(kind string) string {
	// Grammar-only containers are transparent. In particular, Go grammar
	// ABI 15 wraps block children in statement_list without changing the
	// source structure that Mori intends to compare.
	if kind == "statement_list" {
		return ""
	}

	if value, ok := canonicalKinds[kind]; ok {
		return value
	}

	switch {
	case strings.HasSuffix(kind, "_statement"):
		return "statement:" + strings.TrimSuffix(kind, "_statement")
	case strings.HasSuffix(kind, "_expression"):
		return "expression:" + strings.TrimSuffix(kind, "_expression")
	case strings.HasSuffix(kind, "_declaration"):
		return "declaration:" + strings.TrimSuffix(kind, "_declaration")
	case strings.HasSuffix(kind, "_literal"):
		return "literal:" + strings.TrimSuffix(kind, "_literal")
	default:
		return "syntax:" + kind
	}
}

var canonicalKinds = map[string]string{
	// Function boundaries.
	"arrow_function":                 "function",
	"closure_expression":             "function",
	"func_literal":                   "function",
	"function_declaration":           "function",
	"function_definition":            "function",
	"function_expression":            "function",
	"function_item":                  "function",
	"generator_function":             "function",
	"generator_function_declaration": "function",
	"method_declaration":             "function",
	"method_definition":              "function",

	// Parameters, blocks, and bindings.
	"block":                 "block",
	"formal_parameters":     "parameters",
	"parameter":             "parameter",
	"parameter_declaration": "parameter",
	"parameter_list":        "parameters",
	"parameters":            "parameters",
	"required_parameter":    "parameter",
	"optional_parameter":    "parameter",
	"typed_parameter":       "parameter",
	"default_parameter":     "parameter:default",
	"assignment_pattern":    "parameter:default",
	"statement_block":       "block",
	"let_declaration":       "binding",
	"lexical_declaration":   "binding",
	"short_var_declaration": "binding",
	"variable_declaration":  "binding",
	"variable_declarator":   "binding",

	// Flow control.
	"break_statement":    "flow:break",
	"continue_statement": "flow:continue",
	"defer_statement":    "flow:defer",
	"else_clause":        "flow:else",
	"except_clause":      "flow:catch",
	"for_clause":         "flow:loop",
	"for_in_clause":      "flow:loop",
	"for_in_statement":   "flow:loop",
	"for_statement":      "flow:loop",
	"if_expression":      "flow:if",
	"if_statement":       "flow:if",
	"match_arm":          "flow:case",
	"match_block":        "flow:switch",
	"match_expression":   "flow:switch",
	"return_expression":  "flow:return",
	"return_statement":   "flow:return",
	"switch_case":        "flow:case",
	"switch_default":     "flow:case",
	"switch_statement":   "flow:switch",
	"try_expression":     "flow:try",
	"try_statement":      "flow:try",
	"while_statement":    "flow:loop",
	"yield_expression":   "flow:yield",

	// Expressions and data access.
	"argument_list":            "arguments",
	"arguments":                "arguments",
	"array":                    "collection",
	"array_expression":         "collection",
	"array_literal":            "collection",
	"assignment":               "expression:assignment",
	"assignment_expression":    "expression:assignment",
	"assignment_statement":     "expression:assignment",
	"attribute":                "expression:member",
	"await_expression":         "expression:await",
	"binary_expression":        "expression:binary",
	"binary_operator":          "expression:binary",
	"boolean_operator":         "expression:boolean",
	"call":                     "expression:call",
	"call_expression":          "expression:call",
	"comparison_operator":      "expression:comparison",
	"conditional_expression":   "expression:conditional",
	"dictionary":               "collection",
	"element":                  "expression:index",
	"field_expression":         "expression:member",
	"index":                    "expression:index",
	"index_expression":         "expression:index",
	"list":                     "collection",
	"map":                      "collection",
	"member_expression":        "expression:member",
	"new_expression":           "expression:new",
	"object":                   "collection",
	"parenthesized_expression": "expression:group",
	"selector_expression":      "expression:member",
	"slice_expression":         "expression:slice",
	"subscript":                "expression:index",
	"tuple":                    "collection",
	"unary_expression":         "expression:unary",
	"unary_operator":           "expression:unary",

	// Names are deliberately anonymous.
	"field_identifier":              "symbol",
	"identifier":                    "symbol",
	"property_identifier":           "symbol",
	"self":                          "symbol:self",
	"shorthand_property_identifier": "symbol",
	"this":                          "symbol:self",

	// Literal values are reduced to their kind.
	"boolean":                    "literal:boolean",
	"char_literal":               "literal:character",
	"false":                      "literal:boolean",
	"float":                      "literal:number",
	"float_literal":              "literal:number",
	"integer":                    "literal:number",
	"integer_literal":            "literal:number",
	"interpreted_string_literal": "literal:string",
	"none":                       "literal:null",
	"null":                       "literal:null",
	"nil":                        "literal:null",
	"number":                     "literal:number",
	"raw_string_literal":         "literal:string",
	"regex":                      "literal:pattern",
	"regex_pattern":              "literal:pattern",
	"string":                     "literal:string",
	"string_content":             "literal:string-part",
	"string_literal":             "literal:string",
	"template_string":            "literal:string",
	"true":                       "literal:boolean",
}

func canonicalOperator(kind string) string {
	switch kind {
	case "&&", "and":
		return "operator:and"
	case "||", "or":
		return "operator:or"
	case "!", "not":
		return "operator:not"
	case "==", "===":
		return "operator:equal"
	case "!=", "!==":
		return "operator:not-equal"
	case "<", "<=", ">", ">=":
		return "operator:ordered"
	case "in":
		return "operator:membership"
	case "+", "-":
		return "operator:additive"
	case "*", "/", "//", "%":
		return "operator:multiplicative"
	case "**":
		return "operator:power"
	case "&", "|", "^", "<<", ">>":
		return "operator:bitwise"
	case "=", ":=":
		return "operator:assign"
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=":
		return "operator:compound-assign"
	default:
		return ""
	}
}

func canonicalRole(field string) string {
	switch field {
	case "arguments":
		return "arguments"
	case "alternative":
		return "alternative"
	case "body":
		return "body"
	case "condition":
		return "condition"
	case "consequence":
		return "consequence"
	case "function":
		return "callee"
	case "index":
		return "index"
	case "left":
		return "operand"
	case "object", "operand":
		return "receiver"
	case "parameters":
		return "parameters"
	case "right":
		return "operand"
	case "value":
		return "value"
	default:
		return ""
	}
}

func coarseClass(canonical string) string {
	switch {
	case canonical == "function":
		return "function"
	case canonical == "parameters" || strings.HasPrefix(canonical, "parameter"):
		return "parameter"
	case canonical == "block":
		return "block"
	case canonical == "arguments":
		return "arguments"
	case canonical == "binding":
		return "binding"
	case canonical == "collection":
		return "collection"
	case canonical == "symbol" || canonical == "symbol:self":
		return "operand"
	case strings.HasPrefix(canonical, "literal:"):
		return "operand"
	case strings.HasPrefix(canonical, "flow:"):
		return "control"
	case strings.HasPrefix(canonical, "expression:"):
		return "operation"
	case strings.HasPrefix(canonical, "operator:"):
		return "operator"
	default:
		return ""
	}
}

func semanticOperation(node *tree_sitter.Node, source []byte) string {
	kind := node.Kind()
	if kind != "call" && kind != "call_expression" {
		return ""
	}

	callee := node.ChildByFieldName("function")
	if callee == nil {
		callee = node.ChildByFieldName("callee")
	}
	if callee == nil {
		return ""
	}

	name := strings.ToLower(rightmostWord(callee.Utf8Text(source)))
	switch name {
	case "contains", "containskey", "containsvalue", "has", "haskey", "includes", "indexof":
		return "membership"
	case "match", "matches", "matchstring", "search", "test":
		return "pattern-match"
	case "len", "length", "size":
		return "length"
	case "lower", "tolower", "tolowercase":
		return "lowercase"
	case "upper", "toupper", "touppercase":
		return "uppercase"
	case "strip", "trim", "trimspace":
		return "trim"
	case "filter", "where":
		return "filter"
	case "fold", "reduce":
		return "reduce"
	case "map", "select":
		return "map"
	default:
		return ""
	}
}

func rightmostWord(value string) string {
	end := len(value)
	for end > 0 {
		r, size := runeBefore(value, end)
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			break
		}
		end -= size
	}
	start := end
	for start > 0 {
		r, size := runeBefore(value, start)
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		start -= size
	}
	return value[start:end]
}

func runeBefore(value string, index int) (rune, int) {
	r, size := utf8.DecodeLastRuneInString(value[:index])
	return r, size
}

func shouldSkipSubtree(kind string) bool {
	switch kind {
	case "comment",
		"decorator",
		"generic_type",
		"lifetime",
		"lifetime_parameter",
		"primitive_type",
		"type_annotation",
		"type_arguments",
		"type_bound",
		"type_identifier",
		"type_parameter",
		"type_parameters":
		return true
	default:
		return false
	}
}

func addFeature(bag model.FeatureBag, feature string, count int) {
	if feature == "" || count <= 0 {
		return
	}
	bag[feature] += count
}
