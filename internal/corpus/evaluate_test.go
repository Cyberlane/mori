package corpus_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Cyberlane/mori/internal/corpus"
)

func TestRedistributableCorpusExpectations(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Join(filepath.Dir(current), "..", "..", "corpus")
	report, err := corpus.Evaluate(context.Background(), root, filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("corpus violations = %#v", report.Violations)
	}
	if report.CaseCount < 8 || report.Metrics.ActionableGroups == 0 ||
		report.Metrics.FalsePositiveGroups == 0 || len(report.Metrics.PrecisionAtK) < 3 {
		t.Fatalf("corpus report lacks classification breadth: %#v", report)
	}
	first, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal first report: %v", err)
	}
	again, err := corpus.Evaluate(context.Background(), root, filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatalf("Evaluate again: %v", err)
	}
	second, err := json.Marshal(again)
	if err != nil {
		t.Fatalf("Marshal second report: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("corpus report is not deterministic")
	}
}
