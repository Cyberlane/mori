// Package normalize converts grammar-specific trees into shared feature bags.
package normalize

import (
	"context"
	"crypto/sha256"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Cyberlane/mori/internal/model"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Version identifies the normalization contract used to build feature bags.
// Bump it whenever the selected comparison-unit contract, feature vocabulary,
// weights, canonical mappings, or semantic-hint list changes.
const Version = 12

const (
	maxOrderedCallFeatures = 8
	maxOrderedCalleeSlots  = 8
)

// Profile is a normalized, language-neutral view of one syntax fragment.
// It is not the stable content identity exposed in reports.
type Profile struct {
	Features       model.FeatureBag
	TokenCount     int
	LiteralDigests []string
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
	if err := addOrderedFeatures(
		ctx,
		root,
		source,
		isBoundary,
		excludeNestedBoundaries,
		profile.Features,
	); err != nil {
		return Profile{}, err
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
		field := current.node.FieldNameForChild(uint32(childIndex))
		childParent := current.childParent
		if current.node.Kind() == "method_invocation" {
			if current.node.ChildByFieldName("object") != nil &&
				(field == "object" || field == "name") {
				childParent = "expression:member"
			} else if field == "name" {
				field = "function"
			}
		}
		stack = append(stack, frame{
			node:            child,
			parentCanonical: childParent,
			field:           field,
		})
	}

	return profile, nil
}

// BuildSequence creates one bounded comparison profile from an ordered list
// of statement roots. The synthetic block feature distinguishes these opt-in
// partial-function units from independently extracted functions.
func BuildSequence(
	ctx context.Context,
	roots []*tree_sitter.Node,
	source []byte,
	isBoundary func(string) bool,
	excludeNestedBoundaries bool,
) (Profile, error) {
	return BuildCollection(
		ctx, roots, source, isBoundary, excludeNestedBoundaries, "block",
	)
}

// BuildCollection merges ordered syntax roots beneath one synthetic canonical
// node. It is used only when an opt-in comparison unit has no single
// Tree-sitter node spanning exactly the selected roots.
func BuildCollection(
	ctx context.Context,
	roots []*tree_sitter.Node,
	source []byte,
	isBoundary func(string) bool,
	excludeNestedBoundaries bool,
	canonicalRoot string,
) (Profile, error) {
	profile := Profile{Features: make(model.FeatureBag)}
	if len(roots) == 0 {
		return profile, nil
	}
	if canonicalRoot != "" {
		addFeature(profile.Features, "node:"+canonicalRoot, 1)
		profile.TokenCount = 1
	}
	addStatementOrderFeatures(profile.Features, roots, source, isBoundary)
	for _, root := range roots {
		part, err := Build(ctx, root, source, isBoundary, excludeNestedBoundaries)
		if err != nil {
			return Profile{}, err
		}
		for feature, count := range part.Features {
			addFeature(profile.Features, feature, count)
		}
		profile.TokenCount += part.TokenCount
		profile.LiteralDigests = append(profile.LiteralDigests, part.LiteralDigests...)
	}
	return profile, nil
}

type orderedCall struct {
	callee  string
	context string
}

// addOrderedFeatures adds bounded, low-weight evidence that a plain multiset
// cannot represent. Statement features use only canonical shapes. Call-role
// features use fragment-local anonymous slots and never expose callee names or
// name digests in fingerprints or reports.
func addOrderedFeatures(
	ctx context.Context,
	root *tree_sitter.Node,
	source []byte,
	isBoundary func(string) bool,
	excludeNestedBoundaries bool,
	features model.FeatureBag,
) error {
	type frame struct {
		node    *tree_sitter.Node
		context string
	}

	rootID := root.Id()
	stack := []frame{{node: root, context: "linear"}}
	calls := make([]orderedCall, 0)
	callees := make(map[string]struct{})

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := current.node
		if node == nil || node.IsError() || node.IsMissing() || node.IsExtra() {
			continue
		}
		kind := node.Kind()
		if excludeNestedBoundaries && node.Id() != rootID && isBoundary != nil && isBoundary(kind) {
			continue
		}
		if shouldSkipSubtree(kind) {
			continue
		}

		if canonicalNamed(node, source) == "block" {
			addStatementOrderFeatures(features, directOrderedStatements(node), source, isBoundary)
		}
		if canonicalNamed(node, source) == "expression:call" {
			if callee := callCallee(node, source); callee != "" {
				calls = append(calls, orderedCall{callee: callee, context: current.context})
				callees[callee] = struct{}{}
			}
		}

		for index := int(node.ChildCount()) - 1; index >= 0; index-- {
			child := node.Child(uint(index))
			if child == nil {
				continue
			}
			field := node.FieldNameForChild(uint32(index))
			stack = append(stack, frame{
				node:    child,
				context: orderedControlContext(node, field, current.context, source),
			})
		}
	}

	// Anonymous slots are useful only when a fragment has multiple callees.
	// Skip larger call vocabularies rather than letting project-specific names
	// have unbounded influence over a structural score.
	if len(callees) < 2 || len(callees) > maxOrderedCalleeSlots {
		return nil
	}
	orderedCallees := make([]string, 0, len(callees))
	for callee := range callees {
		orderedCallees = append(orderedCallees, callee)
	}
	sort.Strings(orderedCallees)
	slots := make(map[string]int, len(orderedCallees))
	for index, callee := range orderedCallees {
		slots[callee] = index
	}
	for index, call := range calls {
		if index >= maxOrderedCallFeatures {
			break
		}
		addFeature(
			features,
			"ordered:call:"+call.context+":callee-slot-"+strconv.Itoa(slots[call.callee]),
			1,
		)
	}
	return nil
}

func addStatementOrderFeatures(
	features model.FeatureBag,
	statements []*tree_sitter.Node,
	source []byte,
	isBoundary func(string) bool,
) {
	for index, statement := range statements {
		if statement == nil || statement.IsError() || statement.IsMissing() || statement.IsExtra() {
			continue
		}
		if isBoundary != nil && isBoundary(statement.Kind()) {
			continue
		}
		shape := canonicalNamed(statement, source)
		if shape == "" {
			continue
		}
		addFeature(
			features,
			"ordered:statement:"+statementPosition(index, len(statements))+":"+shape,
			1,
		)
	}
}

func directOrderedStatements(block *tree_sitter.Node) []*tree_sitter.Node {
	statements := namedChildren(block)
	if len(statements) == 1 && isTransparentStatementContainer(statements[0].Kind()) {
		return namedChildren(statements[0])
	}
	return statements
}

func namedChildren(node *tree_sitter.Node) []*tree_sitter.Node {
	children := make([]*tree_sitter.Node, 0, node.NamedChildCount())
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if child := node.NamedChild(index); child != nil {
			children = append(children, child)
		}
	}
	return children
}

func isTransparentStatementContainer(kind string) bool {
	return kind == "statement_list" || kind == "statements"
}

func statementPosition(index, count int) string {
	if count == 1 {
		return "only"
	}
	if index == 0 {
		return "first"
	}
	if index == count-1 {
		return "last"
	}
	return "middle"
}

func orderedControlContext(
	parent *tree_sitter.Node,
	field string,
	inherited string,
	source []byte,
) string {
	switch canonicalNamed(parent, source) {
	case "flow:if":
		switch field {
		case "condition":
			return "condition"
		case "alternative":
			return "alternative"
		default:
			return "branch"
		}
	case "flow:loop":
		if field == "condition" {
			return "loop-condition"
		}
		return "loop-body"
	case "flow:switch", "flow:case":
		return "selection"
	case "flow:defer":
		return "deferred"
	default:
		return inherited
	}
}

func callCallee(node *tree_sitter.Node, source []byte) string {
	callee := node.ChildByFieldName("function")
	if callee == nil {
		callee = node.ChildByFieldName("callee")
	}
	if callee == nil && node.Kind() == "method_invocation" {
		callee = node.ChildByFieldName("name")
	}
	if callee == nil && node.Kind() == "invocation" && node.NamedChildCount() > 0 {
		callee = node.NamedChild(0)
	}
	if callee == nil {
		return ""
	}
	return strings.ToLower(rightmostWord(callee.Utf8Text(source)))
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
		canonical = canonicalNamed(node, source)
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
		} else if parentCanonical == "expression:call" && canonical == "arguments" {
			addFeature(profile.Features, "role:arguments>arguments", 1)
		}
		nextParent = canonical
		if strings.HasPrefix(canonical, "literal:") {
			digest := sha256.Sum256(append([]byte(canonical+"\x00"), node.Utf8Text(source)...))
			profile.LiteralDigests = append(profile.LiteralDigests, string(digest[:]))
		}
	}
	if kind == "method_invocation" && node.ChildByFieldName("object") != nil {
		// Java represents a receiver call without the member-access wrapper used
		// by the other code grammars. Add that anonymous structural layer so a
		// receiver and method name retain the same shape without preserving text.
		addFeature(profile.Features, "node:expression:member", 1)
		addFeature(profile.Features, "class:operation", 1)
		addFeature(profile.Features, "edge:expression:call>expression:member", 1)
		addFeature(profile.Features, "role:callee>expression:member", 1)
		profile.TokenCount++
	}

	if operation := semanticOperation(node, source); operation != "" {
		// Semantic operation families are deliberate hints, not claims of
		// behavioral equivalence. A weight of two keeps them useful without
		// overpowering the surrounding tree structure.
		addFeature(profile.Features, "semantic:"+operation, 2)
	}
	if canonical == "operator:membership" && (!node.IsNamed() || kind == "keyword_in") {
		addFeature(profile.Features, "semantic:membership", 2)
	}

	return nextParent, true
}

func canonicalNamed(node *tree_sitter.Node, source []byte) string {
	kind := node.Kind()
	// Grammar-only containers are transparent. In particular, Go grammar
	// ABI 15 wraps block children in statement_list without changing the
	// source structure that Mori intends to compare.
	if kind == "statement_list" || kind == "expression_list" || kind == "value_argument" ||
		kind == "argument" || kind == "parenthesized_expression" ||
		kind == "stmt" || kind == "select_no_parens" || kind == "simple_select" ||
		kind == "a_expr" || kind == "a_expr_prec" || kind == "c_expr" ||
		kind == "ColId" || kind == "ColLabel" || kind == "attr_name" ||
		kind == "indirection" || kind == "indirection_el" || kind == "unreserved_keyword" ||
		strings.HasPrefix(kind, "opt_") {
		return ""
	}
	if kind == "statements" {
		parent := node.Parent()
		if parent != nil && (parent.Kind() == "function_body" || parent.Kind() == "lambda_literal") {
			return ""
		}
		return "block"
	}
	if kind == "control_transfer_statement" {
		return swiftControlTransferKind(node.Utf8Text(source))
	}
	if kind == "binary_expression" && directAssignmentOperator(node) {
		return "expression:assignment"
	}

	if value, ok := canonicalKinds[kind]; ok {
		return value
	}
	if kind == "literal" {
		return sqlLiteralKind(node.Utf8Text(source))
	}
	if strings.HasPrefix(kind, "keyword_") || strings.HasPrefix(kind, "kw_") {
		return ""
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
	"arrow_function":                  "function",
	"accessor_declaration":            "function",
	"anonymous_method_expression":     "function",
	"anonymous_function_expression":   "function",
	"anonymous_function":              "function",
	"closure_expression":              "function",
	"compact_constructor_declaration": "function",
	"constructor_declaration":         "function",
	"conversion_operator_declaration": "function",
	"destructor_declaration":          "function",
	"func_literal":                    "function",
	"function_declaration":            "function",
	"function_definition":             "function",
	"function_expression":             "function",
	"function_item":                   "function",
	"generator_function":              "function",
	"generator_function_declaration":  "function",
	"method_declaration":              "function",
	"method_definition":               "function",
	"init_declaration":                "function",
	"deinit_declaration":              "function",
	"lambda_literal":                  "function",
	"lambda_expression":               "function",
	"lambda":                          "function",
	"local_function_statement":        "function",
	"operator_declaration":            "function",

	// SQL query boundaries and structure.
	"statement":               "query",
	"toplevel_stmt":           "query",
	"select":                  "query:select",
	"SelectStmt":              "query:select",
	"insert":                  "query:insert",
	"InsertStmt":              "query:insert",
	"update":                  "query:update",
	"UpdateStmt":              "query:update",
	"delete":                  "query:delete",
	"DeleteStmt":              "query:delete",
	"cte":                     "query:cte",
	"common_table_expr":       "query:cte",
	"select_expression":       "query:projection",
	"select_clause":           "query:projection",
	"target_list":             "query:projection",
	"term":                    "query:projection-item",
	"target_el":               "query:projection-item",
	"from":                    "query:source",
	"from_clause":             "query:source",
	"relation":                "query:relation",
	"relation_expr":           "query:relation",
	"qualified_name":          "query:relation",
	"join":                    "query:join",
	"join_expr":               "query:join",
	"cross_join":              "query:join:cross",
	"lateral_join":            "query:join:lateral",
	"lateral_cross_join":      "query:join:lateral-cross",
	"where":                   "query:where",
	"where_clause":            "query:where",
	"where_or_current_clause": "query:where",
	"group_by":                "query:group",
	"group_clause":            "query:group",
	"order_by":                "query:order",
	"sort_clause":             "query:order",
	"order_target":            "query:order-target",
	"sortby":                  "query:order-target",
	"limit":                   "query:limit",
	"limit_clause":            "query:limit",
	"offset":                  "query:offset",
	"offset_clause":           "query:offset",
	"returning":               "query:returning",
	"returning_clause":        "query:returning",
	"set_operation":           "query:set-operation",
	"subquery":                "query:subquery",
	"values":                  "query:values",
	"values_clause":           "query:values",
	"assignment_list":         "query:assignments",
	"set_clause_list":         "query:assignments",
	"all_fields":              "query:wildcard",
	"window_clause":           "query:window",
	"window_specification":    "query:window-specification",
	"window_frame":            "query:window-frame",
	"partition_by":            "query:partition",
	"with_clause":             "query:with",

	// SQL expressions and operands.
	"between_expression": "expression:between",
	"case":               "expression:case",
	"when_clause":        "expression:case-branch",
	"cast":               "expression:cast",
	"exists":             "expression:exists",
	"filter_expression":  "expression:filter",
	"invocation":         "expression:call",
	"window_function":    "expression:window-call",
	"field":              "symbol",
	"object_reference":   "symbol",
	"column":             "symbol",
	"columnref":          "symbol",
	"param":              "parameter",
	"AexprConst":         "literal",

	// SQL keyword nodes that carry structure not already represented by a
	// named clause node. Other keyword_* nodes are transparent.
	"keyword_with":      "query:with",
	"keyword_having":    "query:having",
	"keyword_distinct":  "query:distinct",
	"keyword_conflict":  "query:conflict",
	"keyword_duplicate": "query:conflict",
	"keyword_union":     "query:set:union",
	"keyword_except":    "query:set:except",
	"keyword_intersect": "query:set:intersect",
	"keyword_left":      "query:join-kind:left",
	"keyword_right":     "query:join-kind:right",
	"keyword_inner":     "query:join-kind:inner",
	"keyword_full":      "query:join-kind:full",
	"keyword_cross":     "query:join-kind:cross",
	"keyword_lateral":   "query:join-kind:lateral",
	"keyword_and":       "operator:and",
	"keyword_or":        "operator:or",
	"keyword_not":       "operator:not",
	"keyword_in":        "operator:membership",
	"keyword_is":        "operator:is",
	"keyword_like":      "operator:pattern-match",
	"kw_true":           "literal:boolean",
	"kw_false":          "literal:boolean",
	"kw_null":           "literal:null",

	// Parameters, blocks, and bindings.
	"block":                        "block",
	"body":                         "block",
	"compound_statement":           "block",
	"constructor_body":             "block",
	"formal_parameters":            "parameters",
	"formal_parameter":             "parameter",
	"simple_parameter":             "parameter",
	"variadic_parameter":           "parameter",
	"property_promotion_parameter": "parameter",
	"parameter":                    "parameter",
	"parameter_declaration":        "parameter",
	"parameter_list":               "parameters",
	"parameters":                   "parameters",
	"required_parameter":           "parameter",
	"optional_parameter":           "parameter",
	"typed_parameter":              "parameter",
	"default_parameter":            "parameter:default",
	"assignment_pattern":           "parameter:default",
	"statement_block":              "block",
	"function_body":                "block",
	"let_declaration":              "binding",
	"lexical_declaration":          "binding",
	"short_var_declaration":        "binding",
	"variable_declaration":         "binding",
	"variable_declarator":          "binding",
	"local_declaration_statement":  "binding",
	"local_variable_declaration":   "binding",
	"property_declaration":         "binding",
	"property_declarator":          "binding",
	"function_static_declaration":  "binding",
	"static_variable_declaration":  "binding",

	// Shell grammars use different node names for the same variable expansion
	// and word shapes. These aliases retain the surrounding command structure
	// while preventing grammar vocabulary from dominating Bash/Zsh scores.
	"simple_expansion":      "expression:variable",
	"variable_ref":          "expression:variable",
	"simple_variable_name":  "symbol",
	"special_variable_name": "symbol",
	"variable_name":         "symbol",
	"variable":              "symbol",
	"qualified_identifier":  "symbol",
	"name":                  "symbol",
	"scope_identifier":      "symbol",
	"glob_pattern":          "symbol",
	"extglob_pattern":       "symbol",
	"word":                  "symbol",

	// Flow control.
	"break_statement":        "flow:break",
	"continue_statement":     "flow:continue",
	"defer_statement":        "flow:defer",
	"else_clause":            "flow:else",
	"except_clause":          "flow:catch",
	"for_clause":             "flow:loop",
	"for_in_clause":          "flow:loop",
	"for_in_statement":       "flow:loop",
	"for_statement":          "flow:loop",
	"enhanced_for_statement": "flow:loop",
	"foreach_statement":      "flow:loop",
	"if_expression":          "flow:if",
	"if_statement":           "flow:if",
	"match_arm":              "flow:case",
	"match_block":            "flow:switch",
	"match_expression":       "flow:switch",
	"return_expression":      "flow:return",
	"return_statement":       "flow:return",
	"switch_case":            "flow:case",
	"switch_default":         "flow:case",
	"switch_statement":       "flow:switch",
	"try_expression":         "flow:try",
	"try_statement":          "flow:try",
	"while_statement":        "flow:loop",
	"yield_expression":       "flow:yield",
	"catch_clause":           "flow:catch",
	"switch_expression_arm":  "flow:case",
	"switch_rule":            "flow:case",

	// Expressions and data access.
	"argument_list":                     "arguments",
	"arguments":                         "arguments",
	"arrow_expression_clause":           "flow:return",
	"array":                             "collection",
	"array_expression":                  "collection",
	"array_literal":                     "collection",
	"collection":                        "collection",
	"list_expression":                   "collection",
	"list_literal":                      "collection",
	"assignment":                        "expression:assignment",
	"assignment_expression":             "expression:assignment",
	"augmented_assignment_expression":   "expression:assignment",
	"reference_assignment_expression":   "expression:assignment",
	"assignment_statement":              "expression:assignment",
	"attribute":                         "expression:member",
	"await_expression":                  "expression:await",
	"binary_expression":                 "expression:binary",
	"binary_operator":                   "expression:binary",
	"boolean_operator":                  "expression:boolean",
	"conjunction_expression":            "expression:boolean",
	"disjunction_expression":            "expression:boolean",
	"call":                              "expression:call",
	"call_expression":                   "expression:call",
	"function_call_expression":          "expression:call",
	"member_call_expression":            "expression:call",
	"nullsafe_member_call_expression":   "expression:call",
	"scoped_call_expression":            "expression:call",
	"invocation_expression":             "expression:call",
	"method_invocation":                 "expression:call",
	"comparison_operator":               "expression:comparison",
	"additive_expression":               "expression:binary",
	"comparison_expression":             "expression:binary",
	"equality_expression":               "expression:binary",
	"infix_expression":                  "expression:binary",
	"multiplicative_expression":         "expression:binary",
	"conditional_expression":            "expression:conditional",
	"dictionary":                        "collection",
	"element":                           "expression:index",
	"element_access_expression":         "expression:index",
	"array_access":                      "expression:index",
	"field_expression":                  "expression:member",
	"index":                             "expression:index",
	"index_expression":                  "expression:index",
	"list":                              "collection",
	"map":                               "collection",
	"member_expression":                 "expression:member",
	"member_access_expression":          "expression:member",
	"nullsafe_member_access_expression": "expression:member",
	"scoped_property_access_expression": "expression:member",
	"scoped_identifier":                 "expression:member",
	"selection_expression":              "expression:member",
	"navigation_expression":             "expression:member",
	"new_expression":                    "expression:new",
	"object_creation_expression":        "expression:new",
	"array_creation_expression":         "expression:new",
	"object":                            "collection",
	"parenthesized_expression":          "expression:group",
	"selector_expression":               "expression:member",
	"slice_expression":                  "expression:slice",
	"subscript":                         "expression:index",
	"subscript_expression":              "expression:index",
	"tuple":                             "collection",
	"dictionary_literal":                "collection",
	"unary_expression":                  "expression:unary",
	"prefix_unary_expression":           "expression:unary",
	"postfix_unary_expression":          "expression:unary",
	"unary_operator":                    "expression:unary",
	"cast_expression":                   "expression:cast",
	"ternary_expression":                "expression:conditional",
	"awaitable_expression":              "expression:await",

	// Names are deliberately anonymous.
	"field_identifier":              "symbol",
	"identifier":                    "symbol",
	"simple_identifier":             "symbol",
	"property_identifier":           "symbol",
	"self":                          "symbol:self",
	"shorthand_property_identifier": "symbol",
	"this":                          "symbol:self",

	// Literal values are reduced to their kind.
	"boolean":                         "literal:boolean",
	"char_literal":                    "literal:character",
	"false":                           "literal:boolean",
	"float":                           "literal:number",
	"float_literal":                   "literal:number",
	"integer":                         "literal:number",
	"integer_literal":                 "literal:number",
	"interpreted_string_literal":      "literal:string",
	"none":                            "literal:null",
	"null":                            "literal:null",
	"nil":                             "literal:null",
	"number":                          "literal:number",
	"raw_string_literal":              "literal:string",
	"regex":                           "literal:pattern",
	"regex_pattern":                   "literal:pattern",
	"string":                          "literal:string",
	"string_content":                  "literal:string-part",
	"string_literal":                  "literal:string",
	"template_string":                 "literal:string",
	"character_literal":               "literal:character",
	"boolean_literal":                 "literal:boolean",
	"null_literal":                    "literal:null",
	"decimal_integer_literal":         "literal:number",
	"hex_integer_literal":             "literal:number",
	"octal_integer_literal":           "literal:number",
	"binary_integer_literal":          "literal:number",
	"decimal_floating_point_literal":  "literal:number",
	"hex_floating_point_literal":      "literal:number",
	"true":                            "literal:boolean",
	"line_string_literal":             "literal:string",
	"multi_line_string_literal":       "literal:string",
	"value_arguments":                 "arguments",
	"lambda_parameter":                "parameter",
	"lambda_function_type_parameters": "parameters",
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
	case "<", "<=", ">", ">=", "<>":
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

func directAssignmentOperator(node *tree_sitter.Node) bool {
	for index := uint(0); index < node.ChildCount(); index++ {
		child := node.Child(index)
		if child == nil || child.IsNamed() {
			continue
		}
		switch canonicalOperator(child.Kind()) {
		case "operator:assign", "operator:compound-assign":
			return true
		}
	}
	return false
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
	case "left", "lhs":
		return "operand"
	case "object", "operand":
		return "receiver"
	case "parameters":
		return "parameters"
	case "right", "rhs":
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
	case canonical == "query" || strings.HasPrefix(canonical, "query:"):
		return "query"
	default:
		return ""
	}
}

func semanticOperation(node *tree_sitter.Node, source []byte) string {
	kind := node.Kind()
	if kind != "call" && kind != "call_expression" && kind != "invocation" &&
		kind != "invocation_expression" && kind != "method_invocation" &&
		kind != "function_call_expression" && kind != "member_call_expression" &&
		kind != "nullsafe_member_call_expression" && kind != "scoped_call_expression" {
		return ""
	}

	callee := node.ChildByFieldName("function")
	if callee == nil {
		callee = node.ChildByFieldName("callee")
	}
	if callee == nil && kind == "method_invocation" {
		callee = node.ChildByFieldName("name")
	}
	if callee == nil {
		if kind == "invocation" && node.NamedChildCount() > 0 {
			callee = node.NamedChild(0)
		}
		if callee == nil {
			return ""
		}
	}

	name := strings.ToLower(rightmostWord(callee.Utf8Text(source)))
	switch name {
	case "avg", "count", "group_concat", "groupconcat", "max", "min", "sum", "total":
		return "aggregate"
	case "arg", "narg", "slice":
		return "parameter"
	case "contains", "containskey", "containsvalue", "has", "haskey", "includes", "indexof", "str_contains":
		return "membership"
	case "match", "matches", "matchstring", "search", "test":
		return "pattern-match"
	case "len", "length", "size", "strlen":
		return "length"
	case "lower", "strtolower", "tolower", "tolowercase":
		return "lowercase"
	case "strtoupper", "upper", "toupper", "touppercase":
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
		"arraykey",
		"bool",
		"bottom_type",
		"decorator",
		"dynamic",
		"float",
		"generic_type",
		"int",
		"intersection_type",
		"lifetime",
		"lifetime_parameter",
		"primitive_type",
		"mixed",
		"named_type",
		"nonnull",
		"noreturn",
		"nothing",
		"num",
		"optional_type",
		"resource",
		"type_annotation",
		"type_arguments",
		"type_bound",
		"type_identifier",
		"type_parameter",
		"type_parameters",
		"type_specifier",
		"type_list",
		"union_type":
		return true
	case "user_type":
		return true
	default:
		return false
	}
}

func swiftControlTransferKind(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "statement:control-transfer"
	}
	switch fields[0] {
	case "break":
		return "flow:break"
	case "continue":
		return "flow:continue"
	case "fallthrough":
		return "flow:fallthrough"
	case "return":
		return "flow:return"
	case "throw":
		return "flow:throw"
	default:
		return "statement:control-transfer"
	}
}

func sqlLiteralKind(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if value == "?" || isSQLCParameter(lower) {
		return "parameter"
	}
	switch lower {
	case "true", "false":
		return "literal:boolean"
	case "null":
		return "literal:null"
	}
	if value == "" {
		return "literal:scalar"
	}
	first := value[0]
	if first == '\'' || first == '"' ||
		((first == 'e' || first == 'E' || first == 'u' || first == 'U') && strings.Contains(value, "'")) ||
		(first == '$' && strings.Count(value, "$") >= 2) {
		return "literal:string"
	}
	if (first >= '0' && first <= '9') || first == '+' || first == '-' || first == '.' {
		return "literal:number"
	}
	return "literal:scalar"
}

func isSQLCParameter(value string) bool {
	for _, prefix := range []string{"sqlc.arg(", "sqlc.narg("} {
		if strings.HasPrefix(value, prefix) && strings.HasSuffix(value, ")") &&
			strings.TrimSpace(value[len(prefix):len(value)-1]) != "" {
			return true
		}
	}
	return false
}

func addFeature(bag model.FeatureBag, feature string, count int) {
	if feature == "" || count <= 0 {
		return
	}
	bag[feature] += count
}
