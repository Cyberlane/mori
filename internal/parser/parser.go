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
	return FileWithOptions(ctx, file, Options{MinTokens: minTokens})
}

// Coverage records fragment-boundary evidence before the token floor is
// applied. It lets callers distinguish files with no boundaries from files
// whose boundaries were all too small.
type Coverage struct {
	CandidateFragments int
	BelowTokenFloor    int
}

// Options controls opt-in fragment extraction. Defaults preserve the
// function- and query-level comparison contract used by earlier releases.
type Options struct {
	MinTokens        int
	EmbeddedSQL      bool
	SQLDialect       string
	StatementBlocks  bool
	BlockStatements  int
	MaxBlocksPerFunc int
}

// FileWithOptions parses one file using explicit bounded extraction options.
func FileWithOptions(
	ctx context.Context,
	file source.File,
	options Options,
) ([]model.Fragment, []model.Warning) {
	fragments, warnings, _ := FileWithCoverage(ctx, file, options)
	return fragments, warnings
}

// FileWithCoverage parses one file and also returns pre-filter coverage
// evidence for strict file-coverage policies.
func FileWithCoverage(
	ctx context.Context,
	file source.File,
	options Options,
) ([]model.Fragment, []model.Warning, Coverage) {
	content, err := readSource(ctx, file)
	if err != nil {
		return nil, []model.Warning{{
			Path:    file.DisplayPath,
			Message: diagnostic.Message(err),
		}}, Coverage{}
	}

	treeParser := tree_sitter.NewParser()
	defer treeParser.Close()

	if err := treeParser.SetLanguage(file.Language.NewLanguage()); err != nil {
		return nil, []model.Warning{{
			Path:    file.DisplayPath,
			Message: fmt.Sprintf("configure %s parser: %v", file.Language.ID, err),
		}}, Coverage{}
	}

	parserInput := content
	if file.Language.ID == "sql" {
		parserInput = repairSQLParserInput(content)
	} else if file.Language.ID == "hack" {
		parserInput = repairHackParserInput(content)
	}
	tree := treeParser.ParseCtx(ctx, parserInput, nil)
	if tree == nil {
		return nil, []model.Warning{{
			Path:    file.DisplayPath,
			Message: "parser returned no syntax tree",
		}}, Coverage{}
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
	if file.Language.ID == "swift" && tree.RootNode().HasError() {
		if repaired := repairSwiftParserInput(parserInput); repaired != nil {
			repairedTree := treeParser.ParseCtx(ctx, repaired, nil)
			if repairedTree != nil {
				_, originalDiagnostics := parseDiagnostics(tree.RootNode(), 0)
				_, repairedDiagnostics := parseDiagnostics(repairedTree.RootNode(), 0)
				if repairedDiagnostics < originalDiagnostics {
					tree.Close()
					tree = repairedTree
				} else {
					repairedTree.Close()
				}
			}
		}
	}
	defer tree.Close()

	root := tree.RootNode()
	if options.EmbeddedSQL && file.Language.ID == "go" {
		return embeddedSQLFragments(ctx, root, content, file, options)
	}
	coverage := Coverage{}
	warnings := make([]model.Warning, 0, 1)
	fragments := make([]model.Fragment, 0)
	skippedFragments := 0
	if fragmentKind := file.Language.TopLevelFragmentKind(root); fragmentKind != "" {
		coverage.CandidateFragments++
		if root.HasError() {
			skippedFragments++
		} else if included, err := appendFragment(
			ctx,
			root,
			content,
			file,
			options.MinTokens,
			fragmentKind,
			"top-level",
			&fragments,
		); err != nil {
			return nil, []model.Warning{{
				Path:    file.DisplayPath,
				Message: diagnostic.Message(err),
			}}, coverage
		} else if !included {
			coverage.BelowTokenFloor++
		}
	}
	if err := collect(
		ctx,
		root,
		content,
		file,
		options,
		&fragments,
		&warnings,
		&skippedFragments,
		&coverage,
	); err != nil {
		return nil, append(warnings, model.Warning{
			Path:    file.DisplayPath,
			Message: diagnostic.Message(err),
		}), coverage
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
	return fragments, warnings, coverage
}

func collect(
	ctx context.Context,
	node *tree_sitter.Node,
	content []byte,
	file source.File,
	options Options,
	fragments *[]model.Fragment,
	warnings *[]model.Warning,
	skippedFragments *int,
	coverage *Coverage,
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
		accepted := file.Language.AcceptsFragmentBoundary(current)
		if accepted {
			coverage.CandidateFragments++
		}
		if accepted &&
			(current.HasError() || hasInvalidAncestor(current)) {
			(*skippedFragments)++
		} else if accepted {
			included, err := appendFragment(
				ctx,
				current,
				content,
				file,
				options.MinTokens,
				file.Language.FragmentKind,
				"",
				fragments,
			)
			if err != nil {
				return err
			}
			if !included {
				coverage.BelowTokenFloor++
			}
			if options.StatementBlocks {
				blockWarnings, err := appendStatementBlocks(
					ctx, current, content, file, options, fragments,
				)
				if err != nil {
					return err
				}
				*warnings = append(*warnings, blockWarnings...)
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

func appendFragment(
	ctx context.Context,
	node *tree_sitter.Node,
	content []byte,
	file source.File,
	minTokens int,
	fragmentKind string,
	name string,
	fragments *[]model.Fragment,
) (bool, error) {
	profile, err := normalize.Build(
		ctx,
		node,
		content,
		file.Language.IsFragmentBoundary,
		file.Language.ExcludesNestedBoundaries(),
	)
	if err != nil {
		return false, err
	}
	if profile.TokenCount < minTokens {
		return false, nil
	}
	start := node.StartPosition()
	end := node.EndPosition()
	endLine := int(end.Row) + 1
	if end.Column == 0 && end.Row > start.Row {
		endLine = int(end.Row)
	}
	if name == "" {
		name = fragmentName(node, content, fragmentKind)
	}

	*fragments = append(*fragments, model.Fragment{
		Location: model.Location{
			Path:             file.DisplayPath,
			Language:         file.Language.ID,
			LanguageFamily:   file.Language.Family,
			ComparisonDomain: file.Language.ComparisonDomain,
			FragmentKind:     fragmentKind,
			Name:             name,
			StartLine:        int(start.Row) + 1,
			EndLine:          endLine,
		},
		StartByte:      node.StartByte(),
		EndByte:        node.EndByte(),
		TokenCount:     profile.TokenCount,
		FeatureCount:   featureCount(profile.Features),
		Fingerprint:    fingerprint.Bag(profile.Features),
		NestedCount:    profile.Features["node:function:nested"],
		Features:       profile.Features,
		LiteralDigests: append([]string(nil), profile.LiteralDigests...),
	})
	return true, nil
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
			if child.Location.FragmentKind == "block" &&
				parent.Location.FragmentKind == "block" {
				continue
			}
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
	sourceFile source.File,
) (returnBytes []byte, returnErr error) {
	if sourceFile.Content != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if sourceFile.MaxBytes > 0 && int64(len(sourceFile.Content)) > sourceFile.MaxBytes {
			return nil, fmt.Errorf("file exceeded %d-byte limit while reading", sourceFile.MaxBytes)
		}
		return append([]byte(nil), sourceFile.Content...), nil
	}
	file, err := os.Open(sourceFile.Path)
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
	if sourceFile.Info != nil && !os.SameFile(sourceFile.Info, openedInfo) {
		return nil, errors.New("source changed identity after discovery")
	}

	if sourceFile.MaxBytes <= 0 {
		return readAllContext(ctx, file)
	}

	readLimit := sourceFile.MaxBytes
	if sourceFile.MaxBytes < math.MaxInt64 {
		readLimit++
	}
	content, err := readAllContext(ctx, io.LimitReader(file, readLimit))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > sourceFile.MaxBytes {
		return nil, fmt.Errorf("file exceeded %d-byte limit while reading", sourceFile.MaxBytes)
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
