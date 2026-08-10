package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Cyberlane/mori/internal/corpus"
)

func main() {
	root := "corpus"
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/cmd/corpuseval [corpus-root]")
		os.Exit(2)
	}
	if len(os.Args) == 2 {
		root = os.Args[1]
	}
	report, err := corpus.Evaluate(context.Background(), root, filepath.Join(root, "manifest.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "evaluate corpus: %v\n", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "encode corpus report: %v\n", err)
		os.Exit(1)
	}
	if len(report.Violations) != 0 {
		os.Exit(1)
	}
}
