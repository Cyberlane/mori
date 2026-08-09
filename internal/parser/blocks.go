package parser

import (
	"context"
	"fmt"

	"github.com/Cyberlane/mori/internal/fingerprint"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/source"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

var statementContainers = map[string]struct{}{
	"block":                        {},
	"compound_statement":           {},
	"do_group":                     {},
	"function_body":                {},
	"compound_statement_no_always": {},
	"statement_block":              {},
	"statement_list":               {},
	"statements":                   {},
}

func appendStatementBlocks(
	ctx context.Context,
	function *tree_sitter.Node,
	content []byte,
	file source.File,
	options Options,
	fragments *[]model.Fragment,
) ([]model.Warning, error) {
	windows, windowCount := functionStatementWindows(
		function,
		options.BlockStatements,
		options.MaxBlocksPerFunc,
		file.Language.IsFragmentBoundary,
		file.Language.Family == "shell",
	)
	if windowCount == 0 {
		return nil, nil
	}
	if windowCount > options.MaxBlocksPerFunc {
		return []model.Warning{{
			Kind:     "coverage",
			Path:     file.DisplayPath,
			Language: file.Language.ID,
			Message: fmt.Sprintf(
				"statement-block extraction skipped %s at line %d: more than %d windows would be emitted",
				fragmentName(function, content, file.Language.FragmentKind),
				function.StartPosition().Row+1,
				options.MaxBlocksPerFunc,
			),
		}}, nil
	}

	for _, roots := range windows {
		profile, err := normalize.BuildSequence(
			ctx,
			roots,
			content,
			file.Language.IsFragmentBoundary,
			file.Language.ExcludesNestedBoundaries(),
		)
		if err != nil {
			return nil, err
		}
		if profile.TokenCount < options.MinTokens {
			continue
		}
		first := roots[0]
		last := roots[len(roots)-1]
		startPosition := first.StartPosition()
		endPosition := last.EndPosition()
		endLine := int(endPosition.Row) + 1
		if endPosition.Column == 0 && endPosition.Row > startPosition.Row {
			endLine = int(endPosition.Row)
		}
		*fragments = append(*fragments, model.Fragment{
			Location: model.Location{
				Path:             file.DisplayPath,
				Language:         file.Language.ID,
				LanguageFamily:   file.Language.Family,
				ComparisonDomain: file.Language.ComparisonDomain,
				FragmentKind:     "block",
				Name: fmt.Sprintf(
					"block@%d-%d",
					startPosition.Row+1,
					endLine,
				),
				StartLine: int(startPosition.Row) + 1,
				EndLine:   endLine,
			},
			StartByte:      first.StartByte(),
			EndByte:        last.EndByte(),
			TokenCount:     profile.TokenCount,
			FeatureCount:   featureCount(profile.Features),
			Fingerprint:    fingerprint.Bag(profile.Features),
			NestedCount:    profile.Features["node:function:nested"],
			Features:       profile.Features,
			LiteralDigests: append([]string(nil), profile.LiteralDigests...),
		})
	}
	return nil, nil
}

func functionStatementWindows(
	function *tree_sitter.Node,
	size int,
	maxWindows int,
	isBoundary func(string) bool,
	shell bool,
) ([][]*tree_sitter.Node, int) {
	if function == nil {
		return nil, 0
	}
	body := function.ChildByFieldName("body")
	if body == nil {
		for index := uint(0); index < function.NamedChildCount(); index++ {
			candidate := function.NamedChild(index)
			if candidate == nil {
				continue
			}
			if _, ok := statementContainers[candidate.Kind()]; ok {
				body = candidate
			}
		}
	}
	if body == nil {
		return nil, 0
	}
	windows := make([][]*tree_sitter.Node, 0)
	windowCount := 0
	seenContainers := make(map[uintptr]struct{})
	seenWindows := make(map[[2]uint]struct{})
	stack := []*tree_sitter.Node{body}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current != body && isBoundary(current.Kind()) {
			continue
		}
		if isStatementContainer(current.Kind(), shell) {
			container := unwrapStatementContainer(current, shell)
			if _, seen := seenContainers[container.Id()]; !seen {
				seenContainers[container.Id()] = struct{}{}
				statements := directStatements(container)
				for start := 0; start+size <= len(statements); start++ {
					window := append([]*tree_sitter.Node(nil), statements[start:start+size]...)
					span := [2]uint{window[0].StartByte(), window[len(window)-1].EndByte()}
					if _, seen := seenWindows[span]; seen {
						continue
					}
					seenWindows[span] = struct{}{}
					windowCount++
					if windowCount > maxWindows {
						return nil, windowCount
					}
					windows = append(windows, window)
				}
			}
		}
		for index := current.NamedChildCount(); index > 0; index-- {
			child := current.NamedChild(index - 1)
			if child != nil {
				stack = append(stack, child)
			}
		}
	}
	return windows, windowCount
}

func isStatementContainer(kind string, shell bool) bool {
	if _, ok := statementContainers[kind]; ok {
		return true
	}
	if !shell {
		return false
	}
	switch kind {
	case "case_item", "elif_clause", "else_clause", "if_statement", "list":
		return true
	default:
		return false
	}
}

func unwrapStatementContainer(node *tree_sitter.Node, shell bool) *tree_sitter.Node {
	for node.NamedChildCount() == 1 {
		candidate := node.NamedChild(0)
		if candidate == nil || !isStatementContainer(candidate.Kind(), shell) {
			break
		}
		node = candidate
	}
	return node
}

func directStatements(container *tree_sitter.Node) []*tree_sitter.Node {
	statements := make([]*tree_sitter.Node, 0, container.NamedChildCount())
	for index := uint(0); index < container.NamedChildCount(); index++ {
		statement := container.NamedChild(index)
		if statement == nil || statement.IsExtra() || statement.IsError() || statement.IsMissing() {
			continue
		}
		statements = append(statements, statement)
	}
	return statements
}
