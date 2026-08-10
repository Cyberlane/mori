package parser

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Cyberlane/mori/internal/fingerprint"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/source"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

var goDatabaseQueryMethods = map[string]struct{}{
	"Exec":            {},
	"ExecContext":     {},
	"Prepare":         {},
	"PrepareContext":  {},
	"Query":           {},
	"QueryContext":    {},
	"QueryRow":        {},
	"QueryRowContext": {},
}

const (
	maxEmbeddedSQLCandidatesPerFile = 1000
	maxEmbeddedSQLBytes             = 256 * 1024
)

type embeddedSQLCandidate struct {
	literal *tree_sitter.Node
	query   []byte
}

func embeddedSQLFragments(
	ctx context.Context,
	root *tree_sitter.Node,
	content []byte,
	file source.File,
	options Options,
) ([]model.Fragment, []model.Warning, Coverage) {
	candidates := collectGoDatabaseSQL(root, content)
	coverage := Coverage{CandidateFragments: len(candidates)}
	if len(candidates) > maxEmbeddedSQLCandidatesPerFile {
		return nil, []model.Warning{{
			Kind:     "coverage",
			Path:     file.DisplayPath,
			Language: file.Language.ID,
			Message: fmt.Sprintf(
				"embedded-SQL extraction skipped: %d recognized calls exceed per-file limit %d",
				len(candidates),
				maxEmbeddedSQLCandidatesPerFile,
			),
		}}, coverage
	}
	fragments := make([]model.Fragment, 0, len(candidates))
	warnings := make([]model.Warning, 0)
	for _, candidate := range candidates {
		if len(candidate.query) > maxEmbeddedSQLBytes {
			warnings = append(warnings, model.Warning{
				Kind:     "coverage",
				Path:     file.DisplayPath,
				Language: file.Language.ID,
				Message: fmt.Sprintf(
					"embedded SQL at host line %d is %d bytes; per-query limit is %d bytes",
					candidate.literal.StartPosition().Row+1,
					len(candidate.query),
					maxEmbeddedSQLBytes,
				),
			})
			continue
		}
		parsed, warning, err := parseEmbeddedSQLCandidate(ctx, candidate, content, file, options)
		if err != nil {
			return nil, []model.Warning{{Path: file.DisplayPath, Message: err.Error()}}, coverage
		}
		fragments = append(fragments, parsed...)
		if len(parsed) == 0 && (warning == nil || warning.SkippedFragments == 0) {
			coverage.BelowTokenFloor++
		}
		if warning != nil {
			warnings = append(warnings, *warning)
		}
	}
	if root != nil && root.HasError() {
		diagnostics, total := parseDiagnostics(root, 5)
		warnings = append(warnings, model.Warning{
			Kind:             "parse",
			Path:             file.DisplayPath,
			Language:         file.Language.ID,
			Message:          "host syntax tree contains parse errors; embedded-SQL coverage may be incomplete",
			TotalDiagnostics: total,
			Diagnostics:      diagnostics,
		})
	}
	return fragments, warnings, coverage
}

func collectGoDatabaseSQL(root *tree_sitter.Node, content []byte) []embeddedSQLCandidate {
	if root == nil {
		return nil
	}
	candidates := make([]embeddedSQLCandidate, 0)
	cursor := root.Walk()
	defer cursor.Close()
	for {
		current := cursor.Node()
		if current.Kind() == "call_expression" {
			if candidate, ok := goDatabaseSQLCandidate(current, content); ok {
				candidates = append(candidates, candidate)
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
				return candidates
			}
		}
	}
}

func goDatabaseSQLCandidate(
	call *tree_sitter.Node,
	content []byte,
) (embeddedSQLCandidate, bool) {
	function := call.ChildByFieldName("function")
	arguments := call.ChildByFieldName("arguments")
	if function == nil || arguments == nil || function.Kind() != "selector_expression" {
		return embeddedSQLCandidate{}, false
	}
	field := function.ChildByFieldName("field")
	if field == nil {
		return embeddedSQLCandidate{}, false
	}
	if _, ok := goDatabaseQueryMethods[field.Utf8Text(content)]; !ok {
		return embeddedSQLCandidate{}, false
	}
	var literal *tree_sitter.Node
	for index := uint(0); index < arguments.NamedChildCount(); index++ {
		candidate := arguments.NamedChild(index)
		if candidate == nil || candidate.IsExtra() {
			continue
		}
		if candidate.Kind() == "raw_string_literal" ||
			candidate.Kind() == "interpreted_string_literal" {
			literal = candidate
			break
		}
	}
	if literal == nil {
		return embeddedSQLCandidate{}, false
	}
	decoded, err := strconv.Unquote(literal.Utf8Text(content))
	if err != nil {
		return embeddedSQLCandidate{}, false
	}
	return embeddedSQLCandidate{literal: literal, query: []byte(decoded)}, true
}

func parseEmbeddedSQLCandidate(
	ctx context.Context,
	candidate embeddedSQLCandidate,
	hostContent []byte,
	file source.File,
	options Options,
) ([]model.Fragment, *model.Warning, error) {
	specID := "sql"
	if options.SQLDialect == language.SQLDialectPostgreSQL {
		specID = "postgresql"
	}
	spec, ok := language.Lookup(specID)
	if !ok {
		return nil, nil, fmt.Errorf("embedded SQL parser %q is unavailable", specID)
	}
	treeParser := tree_sitter.NewParser()
	defer treeParser.Close()
	if err := treeParser.SetLanguage(spec.NewLanguage()); err != nil {
		return nil, nil, fmt.Errorf("configure %s parser: %w", spec.ID, err)
	}
	parserInput := candidate.query
	if spec.ID == "sql" {
		parserInput = repairSQLParserInput(parserInput)
	}
	tree := treeParser.ParseCtx(ctx, parserInput, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("embedded SQL parser returned no syntax tree")
	}
	defer tree.Close()

	boundaries := acceptedBoundaries(tree.RootNode(), spec)
	valid := make([]*tree_sitter.Node, 0, len(boundaries))
	for _, boundary := range boundaries {
		if boundary.HasError() || hasInvalidAncestor(boundary) {
			continue
		}
		valid = append(valid, boundary)
	}
	fragments := make([]model.Fragment, 0, 1)
	if len(valid) > 0 {
		canonicalRoot := ""
		if len(valid) > 1 {
			canonicalRoot = "query:batch"
		}
		profile, err := normalize.BuildCollection(
			ctx,
			valid,
			parserInput,
			spec.IsFragmentBoundary,
			spec.ExcludesNestedBoundaries(),
			canonicalRoot,
		)
		if err != nil {
			return nil, nil, err
		}
		if profile.TokenCount >= options.MinTokens {
			start := candidate.literal.StartPosition()
			end := candidate.literal.EndPosition()
			endLine := int(end.Row) + 1
			if end.Column == 0 && end.Row > start.Row {
				endLine = int(end.Row)
			}
			fragment := model.Fragment{
				Location: model.Location{
					Path:             file.DisplayPath,
					Language:         spec.ID,
					LanguageFamily:   spec.Family,
					ComparisonDomain: spec.ComparisonDomain,
					FragmentKind:     spec.FragmentKind,
					Name:             fmt.Sprintf("embedded-query@%d", start.Row+1),
					StartLine:        int(start.Row) + 1,
					EndLine:          endLine,
				},
				StartByte:      candidate.literal.StartByte(),
				EndByte:        candidate.literal.EndByte(),
				TokenCount:     profile.TokenCount,
				FeatureCount:   featureCount(profile.Features),
				Fingerprint:    fingerprint.Bag(profile.Features),
				Features:       profile.Features,
				LiteralDigests: append([]string(nil), profile.LiteralDigests...),
			}
			annotateEmbeddedSQLParent(ctx, &fragment, candidate.literal, hostContent, file)
			fragments = append(fragments, fragment)
		}
	}
	if !tree.RootNode().HasError() {
		return fragments, nil, nil
	}
	_, total := parseDiagnostics(tree.RootNode(), 0)
	return fragments, &model.Warning{
		Kind:             "parse",
		Path:             file.DisplayPath,
		Language:         spec.ID,
		Message:          fmt.Sprintf("embedded SQL at host line %d contains parse errors; comparison coverage may be incomplete", candidate.literal.StartPosition().Row+1),
		TotalDiagnostics: total,
		SkippedFragments: len(boundaries) - len(valid),
	}, nil
}

func acceptedBoundaries(root *tree_sitter.Node, spec language.Spec) []*tree_sitter.Node {
	if root == nil {
		return nil
	}
	boundaries := make([]*tree_sitter.Node, 0)
	cursor := root.Walk()
	defer cursor.Close()
	for {
		current := cursor.Node()
		if spec.AcceptsFragmentBoundary(current) {
			boundaries = append(boundaries, current)
		}
		if cursor.GotoFirstChild() {
			continue
		}
		for {
			if cursor.GotoNextSibling() {
				break
			}
			if !cursor.GotoParent() {
				return boundaries
			}
		}
	}
}

func annotateEmbeddedSQLParent(
	ctx context.Context,
	fragment *model.Fragment,
	literal *tree_sitter.Node,
	content []byte,
	file source.File,
) {
	for parent := literal.Parent(); parent != nil; parent = parent.Parent() {
		if !file.Language.AcceptsFragmentBoundary(parent) {
			continue
		}
		profile, err := normalize.Build(
			ctx,
			parent,
			content,
			file.Language.IsFragmentBoundary,
			file.Language.ExcludesNestedBoundaries(),
		)
		if err != nil {
			return
		}
		start := parent.StartPosition()
		end := parent.EndPosition()
		endLine := int(end.Row) + 1
		if end.Column == 0 && end.Row > start.Row {
			endLine = int(end.Row)
		}
		location := model.Location{
			Path:             file.DisplayPath,
			Language:         file.Language.ID,
			LanguageFamily:   file.Language.Family,
			ComparisonDomain: file.Language.ComparisonDomain,
			FragmentKind:     file.Language.FragmentKind,
			Name:             fragmentName(parent, content, file.Language.FragmentKind),
			StartLine:        int(start.Row) + 1,
			EndLine:          endLine,
		}
		fragment.Parent = &location
		fragment.ParentID = fingerprint.Bag(profile.Features)
		fragment.NestingDepth = 1
		return
	}
}
