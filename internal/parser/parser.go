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
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/source"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// File parses one file and returns every function-like fragment that meets the
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

	tree := treeParser.ParseCtx(ctx, content, nil)
	if tree == nil {
		return nil, []model.Warning{{
			Path:    file.DisplayPath,
			Message: "parser returned no syntax tree",
		}}
	}
	defer tree.Close()

	root := tree.RootNode()
	warnings := make([]model.Warning, 0, 1)
	if root.HasError() {
		warnings = append(warnings, model.Warning{
			Path:    file.DisplayPath,
			Message: "syntax tree contains parse errors; invalid fragments were skipped",
		})
	}

	fragments := make([]model.Fragment, 0)
	if err := collect(ctx, root, content, file, minTokens, &fragments); err != nil {
		return nil, append(warnings, model.Warning{
			Path:    file.DisplayPath,
			Message: diagnostic.Message(err),
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
		if file.Language.IsFunction(current.Kind()) && !current.HasError() {
			fingerprint, err := normalize.Build(
				ctx,
				current,
				content,
				file.Language.IsFunction,
			)
			if err != nil {
				return err
			}
			if fingerprint.TokenCount >= minTokens {
				start := current.StartPosition()
				end := current.EndPosition()
				endLine := int(end.Row) + 1
				if end.Column == 0 && end.Row > start.Row {
					endLine = int(end.Row)
				}

				*fragments = append(*fragments, model.Fragment{
					Location: model.Location{
						Path:      file.DisplayPath,
						Language:  file.Language.ID,
						Name:      fragmentName(current, content),
						StartLine: int(start.Row) + 1,
						EndLine:   endLine,
					},
					TokenCount:   fingerprint.TokenCount,
					FeatureCount: featureCount(fingerprint.Features),
					Features:     fingerprint.Features,
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

func fragmentName(node *tree_sitter.Node, content []byte) string {
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
