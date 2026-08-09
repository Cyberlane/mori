// Package parser turns supported source files into normalized fragments.
package parser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/Cyberlane/mori/internal/diagnostic"
	"github.com/Cyberlane/mori/internal/fingerprint"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/source"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// File parses one file and returns every source fragment that meets the
// minimum normalized token count.
func File(
	ctx context.Context,
	file source.File,
	minTokens int,
) ([]model.Fragment, []model.Warning) {
	content, err := readSource(ctx, file.Path, file.MaxBytes, file.Info)
	if err != nil {
		return nil, []model.Warning{{
			Path:    file.DisplayPath,
			Message: diagnostic.Message(err),
		}}
	}

	treeParser := tree_sitter.NewParser()
	defer treeParser.Close()

	if err := treeParser.SetLanguage(file.Language.NewLanguage()); err != nil {
		return nil, []model.Warning{{
			Path:    file.DisplayPath,
			Message: fmt.Sprintf("configure %s parser: %v", file.Language.ID, err),
		}}
	}

	parserInput := content
	if file.Language.ID == "sql" {
		parserInput = repairSQLParserInput(content)
	}
	tree := treeParser.ParseCtx(ctx, parserInput, nil)
	if tree == nil {
		return nil, []model.Warning{{
			Path:    file.DisplayPath,
			Message: "parser returned no syntax tree",
		}}
	}
	if file.Language.ID == "tsx" || file.Language.ID == "javascript" {
		if repaired := repairJSXParserInput(tree.RootNode(), parserInput); repaired != nil {
			repairedTree := treeParser.ParseCtx(ctx, repaired, nil)
			if repairedTree != nil {
				tree.Close()
				tree = repairedTree
			}
		}
	}
	defer tree.Close()

	root := tree.RootNode()
	warnings := make([]model.Warning, 0, 1)
	fragments := make([]model.Fragment, 0)
	skippedFragments := 0
	if err := collect(
		ctx,
		root,
		content,
		file,
		minTokens,
		&fragments,
		&skippedFragments,
	); err != nil {
		return nil, append(warnings, model.Warning{
			Path:    file.DisplayPath,
			Message: diagnostic.Message(err),
		})
	}
	annotateNesting(fragments)
	if root.HasError() {
		diagnostics, total := parseDiagnostics(root, 5)
		warnings = append(warnings, model.Warning{
			Kind:             "parse",
			Path:             file.DisplayPath,
			Language:         file.Language.ID,
			Message:          "syntax tree contains parse errors; comparison coverage may be incomplete",
			TotalDiagnostics: total,
			SkippedFragments: skippedFragments,
			Diagnostics:      diagnostics,
		})
	}
	return fragments, warnings
}

func collect(
	ctx context.Context,
	node *tree_sitter.Node,
	content []byte,
	file source.File,
	minTokens int,
	fragments *[]model.Fragment,
	skippedFragments *int,
) error {
	if node == nil {
		return nil
	}

	cursor := node.Walk()
	defer cursor.Close()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := cursor.Node()
		if file.Language.AcceptsFragmentBoundary(current) &&
			(current.HasError() || hasInvalidAncestor(current)) {
			(*skippedFragments)++
		} else if file.Language.AcceptsFragmentBoundary(current) {
			profile, err := normalize.Build(
				ctx,
				current,
				content,
				file.Language.IsFragmentBoundary,
				file.Language.ExcludesNestedBoundaries(),
			)
			if err != nil {
				return err
			}
			if profile.TokenCount >= minTokens {
				start := current.StartPosition()
				end := current.EndPosition()
				endLine := int(end.Row) + 1
				if end.Column == 0 && end.Row > start.Row {
					endLine = int(end.Row)
				}

				*fragments = append(*fragments, model.Fragment{
					Location: model.Location{
						Path:             file.DisplayPath,
						Language:         file.Language.ID,
						LanguageFamily:   file.Language.Family,
						ComparisonDomain: file.Language.ComparisonDomain,
						FragmentKind:     file.Language.FragmentKind,
						Name:             fragmentName(current, content, file.Language.FragmentKind),
						StartLine:        int(start.Row) + 1,
						EndLine:          endLine,
					},
					StartByte:    current.StartByte(),
					EndByte:      current.EndByte(),
					TokenCount:   profile.TokenCount,
					FeatureCount: featureCount(profile.Features),
					Fingerprint:  fingerprint.Bag(profile.Features),
					NestedCount:  profile.Features["node:function:nested"],
					Features:     profile.Features,
				})
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
				return nil
			}
		}
	}
}

func hasInvalidAncestor(node *tree_sitter.Node) bool {
	if node == nil {
		return false
	}
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if parent.IsError() || parent.IsMissing() {
			return true
		}
	}
	return false
}

func parseDiagnostics(root *tree_sitter.Node, limit int) ([]model.ParseDiagnostic, int) {
	if root == nil {
		return []model.ParseDiagnostic{}, 0
	}
	diagnostics := make([]model.ParseDiagnostic, 0, limit)
	total := 0
	var narrowestErrored *model.ParseDiagnostic
	narrowestSpan := uint(math.MaxUint)
	cursor := root.Walk()
	defer cursor.Close()

	for {
		current := cursor.Node()
		if current.HasError() {
			span := current.EndByte() - current.StartByte()
			if span < narrowestSpan {
				start := current.StartPosition()
				end := current.EndPosition()
				narrowestSpan = span
				narrowestErrored = &model.ParseDiagnostic{
					NodeKind:    current.Kind(),
					StartLine:   int(start.Row) + 1,
					StartColumn: int(start.Column) + 1,
					EndLine:     int(end.Row) + 1,
					EndColumn:   int(end.Column) + 1,
				}
			}
		}
		if current.IsError() || current.IsMissing() {
			total++
			if len(diagnostics) < limit {
				start := current.StartPosition()
				end := current.EndPosition()
				diagnostics = append(diagnostics, model.ParseDiagnostic{
					NodeKind:    current.Kind(),
					StartLine:   int(start.Row) + 1,
					StartColumn: int(start.Column) + 1,
					EndLine:     int(end.Row) + 1,
					EndColumn:   int(end.Column) + 1,
				})
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
				if total == 0 && narrowestErrored != nil {
					return []model.ParseDiagnostic{*narrowestErrored}, 1
				}
				return diagnostics, total
			}
		}
	}
}

func annotateNesting(fragments []model.Fragment) {
	parents := make([]int, len(fragments))
	for index := range parents {
		parents[index] = -1
	}
	for childIndex := range fragments {
		bestSpan := ^uint(0)
		for parentIndex := range fragments {
			if childIndex == parentIndex {
				continue
			}
			child := fragments[childIndex]
			parent := fragments[parentIndex]
			if parent.StartByte > child.StartByte || parent.EndByte < child.EndByte ||
				(parent.StartByte == child.StartByte && parent.EndByte == child.EndByte) {
				continue
			}
			span := parent.EndByte - parent.StartByte
			if span < bestSpan {
				bestSpan = span
				parents[childIndex] = parentIndex
			}
		}
	}

	depths := make([]int, len(fragments))
	var depth func(int) int
	depth = func(index int) int {
		if parents[index] < 0 {
			return 0
		}
		if depths[index] > 0 {
			return depths[index]
		}
		depths[index] = depth(parents[index]) + 1
		return depths[index]
	}
	for index := range fragments {
		parentIndex := parents[index]
		if parentIndex < 0 {
			continue
		}
		fragments[index].NestingDepth = depth(index)
		parentLocation := fragments[parentIndex].Location
		fragments[index].Parent = &parentLocation
		fragments[index].ParentID = fragments[parentIndex].Fingerprint
	}
}

func fragmentName(node *tree_sitter.Node, content []byte, fragmentKind string) string {
	if fragmentKind == "query" {
		if name := sqlcQueryName(node, content); name != "" {
			return name
		}
		return fmt.Sprintf("query@%d", node.StartPosition().Row+1)
	}
	if node.Kind() == "deinit_declaration" {
		return "deinit"
	}
	if name := node.ChildByFieldName("name"); name != nil {
		if value := cleanName(name.Utf8Text(content)); value != "" {
			return value
		}
	}

	parent := node.Parent()
	if parent != nil {
		for _, field := range []string{"name", "left", "key"} {
			candidate := parent.ChildByFieldName(field)
			if candidate == nil {
				continue
			}
			if value := cleanName(candidate.Utf8Text(content)); value != "" {
				return value
			}
		}
	}

	return fmt.Sprintf("anonymous@%d", node.StartPosition().Row+1)
}

func sqlcQueryName(node *tree_sitter.Node, content []byte) string {
	if node == nil || node.StartByte() > uint(len(content)) {
		return ""
	}
	prefix := strings.TrimRight(string(content[:node.StartByte()]), " \t")
	switch {
	case strings.HasSuffix(prefix, "\r\n"):
		prefix = strings.TrimSuffix(prefix, "\r\n")
	case strings.HasSuffix(prefix, "\n"):
		prefix = strings.TrimSuffix(prefix, "\n")
	default:
		return ""
	}
	lineStart := strings.LastIndexByte(prefix, '\n') + 1
	line := strings.TrimSuffix(prefix[lineStart:], "\r")
	const marker = "-- name: "
	if !strings.HasPrefix(line, marker) {
		return ""
	}
	parts := strings.Split(line[len(marker):], " ")
	if len(parts) != 2 || !validSQLCIdentifier(parts[0]) ||
		len(parts[1]) < 2 || parts[1][0] != ':' || !validSQLCIdentifier(parts[1][1:]) {
		return ""
	}
	return parts[0]
}

func validSQLCIdentifier(value string) bool {
	if value == "" || !isASCIIIdentifierStart(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isASCIIIdentifierStart(character) && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func isASCIIIdentifierStart(character byte) bool {
	return (character >= 'A' && character <= 'Z') ||
		(character >= 'a' && character <= 'z') ||
		character == '_'
}

func cleanName(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 80 {
		return string(runes[:77]) + "..."
	}
	return value
}

func readSource(
	ctx context.Context,
	path string,
	maxBytes int64,
	expectedInfo os.FileInfo,
) (returnBytes []byte, returnErr error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil && returnErr == nil {
			returnErr = err
		}
	}()

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() {
		return nil, errors.New("source is no longer a regular file")
	}
	if expectedInfo != nil && !os.SameFile(expectedInfo, openedInfo) {
		return nil, errors.New("source changed identity after discovery")
	}

	if maxBytes <= 0 {
		return readAllContext(ctx, file)
	}

	readLimit := maxBytes
	if maxBytes < math.MaxInt64 {
		readLimit++
	}
	content, err := readAllContext(ctx, io.LimitReader(file, readLimit))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxBytes {
		return nil, fmt.Errorf("file exceeded %d-byte limit while reading", maxBytes)
	}
	return content, nil
}

func readAllContext(ctx context.Context, reader io.Reader) ([]byte, error) {
	buffer := make([]byte, 0, 32*1024)
	chunk := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, err := reader.Read(chunk)
		buffer = append(buffer, chunk[:count]...)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return buffer, nil
			}
			return nil, err
		}
	}
}

func featureCount(features model.FeatureBag) int {
	total := 0
	for _, count := range features {
		total += count
	}
	return total
}
