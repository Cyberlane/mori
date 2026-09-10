package parser

import (
	"github.com/Cyberlane/mori/internal/language"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	"testing"
)

func TestCDanglingElseStillBindsNearestIf(t *testing.T) {
	t.Parallel()
	spec, _ := language.Detect("input.c")
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(spec.NewLanguage()); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse([]byte(`int value(int a, int b) { if (a) if (b) return 1; else return 2; return 0; }`), nil)
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse = %s", root.ToSexp())
	}
	outer := root.NamedChild(0).ChildByFieldName("body").NamedChild(0)
	inner := outer.ChildByFieldName("consequence")
	if outer.Kind() != "if_statement" || inner == nil || inner.Kind() != "if_statement" || outer.ChildByFieldName("alternative") != nil || inner.ChildByFieldName("alternative") == nil {
		t.Fatalf("else association changed: %s", root.ToSexp())
	}
}
