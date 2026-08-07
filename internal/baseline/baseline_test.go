package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
)

func TestLoadRejectsMissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load error = %v, want not-exist error", err)
	}
}

func TestLoadRejectsMalformedAndMismatchedFiles(t *testing.T) {
	t.Parallel()

	malformed := filepath.Join(t.TempDir(), "malformed.json")
	writeFixture(t, malformed, "{")
	if _, err := Load(malformed); err == nil || !strings.Contains(err.Error(), "decode baseline") {
		t.Fatalf("malformed Load error = %v", err)
	}

	mismatched := filepath.Join(t.TempDir(), "mismatched.json")
	writeFixture(t, mismatched, `{"schema_version":1,"normalization_version":99,"entries":[]}`)
	if _, err := Load(mismatched); err == nil || !strings.Contains(err.Error(), "run baseline update") {
		t.Fatalf("mismatched Load error = %v", err)
	}
}

func TestLoadToleratesUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline.json")
	writeFixture(t, path, `{
  "schema_version": 1,
  "mori_version": "0.2.1",
  "normalization_version": 1,
  "threshold": 0.7,
  "future_field": true,
  "entries": [{
    "id": "accepted",
    "similarity": 0.8,
    "left": {"path":"left.go","language":"go","name":"Left","start_line":1,"end_line":2},
    "right": {"path":"right.py","language":"python","name":"right","start_line":3,"end_line":4},
    "future_entry_field": "ignored"
  }]
}`)

	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !set.Has("accepted") || set.Has("missing") {
		t.Fatalf("baseline membership is incorrect")
	}
}

func TestWriteIsDeterministicAndRoundTrips(t *testing.T) {
	t.Parallel()

	report := model.Report{
		Threshold: 0.7,
		Matches: []model.Match{
			{
				ID:         "bbbb",
				Similarity: 0.71,
				Left:       model.FragmentSummary{Location: location("b.go")},
				Right:      model.FragmentSummary{Location: location("b.py")},
			},
			{
				ID:         "aaaa",
				Similarity: 0.82,
				Left:       model.FragmentSummary{Location: location("a.go")},
				Right:      model.FragmentSummary{Location: location("a.py")},
			},
		},
	}
	path := filepath.Join(t.TempDir(), "mori-baseline.json")
	if err := Write(path, report); err != nil {
		t.Fatalf("Write: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Index(string(content), `"id": "aaaa"`) > strings.Index(string(content), `"id": "bbbb"`) {
		t.Fatalf("entries are not sorted by ID:\n%s", content)
	}
	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Write: %v", err)
	}
	if !set.Has("aaaa") || !set.Has("bbbb") {
		t.Fatal("round-tripped entries are missing")
	}
}

func TestWriteRejectsTruncatedReport(t *testing.T) {
	t.Parallel()

	err := Write(filepath.Join(t.TempDir(), "baseline.json"), model.Report{Truncated: true})
	if err == nil || !strings.Contains(err.Error(), "truncated report") {
		t.Fatalf("Write error = %v, want truncated-report error", err)
	}
}

func TestStaleSortsEntriesNotInReport(t *testing.T) {
	t.Parallel()

	set := Set{entries: map[string]Entry{
		"bbbb": {ID: "bbbb"},
		"aaaa": {ID: "aaaa"},
		"keep": {ID: "keep"},
	}}
	stale := Stale(set, model.Report{Matches: []model.Match{{ID: "keep"}}})
	if len(stale) != 2 || stale[0].ID != "aaaa" || stale[1].ID != "bbbb" {
		t.Fatalf("stale = %#v, want sorted aaaa/bbbb", stale)
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "duplicate.json")
	writeFixture(t, path, `{"schema_version":1,"normalization_version":1,"entries":[{"id":"same"},{"id":"same"}]}`)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "duplicate ID") {
		t.Fatalf("duplicate Load error = %v", err)
	}
}

func location(path string) model.Location {
	return model.Location{Path: path, Language: "go", Name: "function", StartLine: 1, EndLine: 2}
}

func writeFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if normalize.Version != 1 {
		t.Fatalf("test fixture version = %d, want 1", normalize.Version)
	}
}
