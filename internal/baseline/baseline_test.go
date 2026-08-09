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
	writeFixture(t, mismatched, `{"schema_version":2,"identity_scope":"content","normalization_version":1,"entries":[]}`)
	if _, err := Load(mismatched); err == nil || !strings.Contains(err.Error(), "run baseline update") {
		t.Fatalf("mismatched Load error = %v", err)
	}
}

func TestLoadSchemaOneAsContentScopeAndToleratesUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline.json")
	writeFixture(t, path, `{
  "schema_version": 1,
  "mori_version": "0.3.0",
  "normalization_version": 6,
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
	if set.Scope() != ScopeContent || !set.Match("accepted", location("new.go"), location("copy.py")) {
		t.Fatalf("schema-one baseline was not loaded as content scope")
	}
}

func TestWriteIsDeterministicAndRoundTrips(t *testing.T) {
	t.Parallel()

	report := model.Report{
		Threshold: 0.7,
		Groups: []model.MatchGroup{
			matchGroup("bbbb", "b.go", "b.py"),
			matchGroup("aaaa", "a.go", "a.py"),
		},
	}
	path := filepath.Join(t.TempDir(), "mori-baseline.json")
	if err := Write(path, report, ScopeContent); err != nil {
		t.Fatalf("Write: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(content), `"schema_version": 2`) ||
		!strings.Contains(string(content), `"identity_scope": "content"`) {
		t.Fatalf("baseline schema/scope missing:\n%s", content)
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

func TestPathScopeDoesNotSuppressCopiedLocation(t *testing.T) {
	t.Parallel()

	report := model.Report{Groups: []model.MatchGroup{matchGroup("same", "left.go", "right.go")}}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := Write(path, report, ScopePath); err != nil {
		t.Fatalf("Write: %v", err)
	}
	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !set.Match("same", location("left.go"), location("right.go")) {
		t.Fatal("reviewed path pair was not suppressed")
	}
	if set.Match("same", location("left.go"), location("copied.go")) {
		t.Fatal("new copied path was suppressed by path scope")
	}
}

func TestPathScopeUsesExactScoredPairsInsteadOfOccurrenceCrossProduct(t *testing.T) {
	t.Parallel()

	left := location("left.go")
	rightA := location("right.ts")
	rightB := location("right.tsx")
	group := model.MatchGroup{
		ID: "same",
		Profiles: []model.FragmentProfile{{
			Fingerprint: "same",
			Occurrences: []model.FragmentSummary{
				{Location: left}, {Location: rightA}, {Location: rightB},
			},
		}},
		PathPairs: []model.LocationPair{
			{Left: left, Right: rightA},
			{Left: left, Right: rightB},
		},
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := Write(path, model.Report{Groups: []model.MatchGroup{group}}, ScopePath); err != nil {
		t.Fatalf("Write: %v", err)
	}
	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !set.Match("same", left, rightA) || !set.Match("same", left, rightB) {
		t.Fatal("exact scored path pairs were not retained")
	}
	if set.Match("same", rightA, rightB) {
		t.Fatal("unscored occurrence cross-product pair was accepted")
	}
}

func TestWriteRejectsTruncatedReport(t *testing.T) {
	t.Parallel()

	err := Write(
		filepath.Join(t.TempDir(), "baseline.json"),
		model.Report{Truncated: true},
		ScopeContent,
	)
	if err == nil || !strings.Contains(err.Error(), "truncated report") {
		t.Fatalf("Write error = %v, want truncated-report error", err)
	}
}

func TestPruneKeepsCurrentEntriesWithoutAddingNewOnes(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "baseline.json")
	set := Set{scope: ScopeContent, entries: map[string]Entry{
		"keep": {ID: "keep", Note: "reviewed"},
		"drop": {ID: "drop"},
	}}
	report := model.Report{Threshold: 0.8, Groups: []model.MatchGroup{
		matchGroup("keep", "keep.go", "keep.py"),
		matchGroup("new", "new.go", "new.py"),
	}}
	if err := Prune(path, set, report); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	pruned, err := Load(path)
	if err != nil {
		t.Fatalf("Load pruned baseline: %v", err)
	}
	if !pruned.Has("keep") || pruned.Has("drop") || pruned.Has("new") {
		t.Fatal("prune did not retain exactly the current accepted entry")
	}
}

func TestStaleSortsEntriesNotInReport(t *testing.T) {
	t.Parallel()

	set := Set{scope: ScopeContent, entries: map[string]Entry{
		"bbbb": {ID: "bbbb"},
		"aaaa": {ID: "aaaa"},
		"keep": {ID: "keep"},
	}}
	stale := Stale(set, model.Report{Groups: []model.MatchGroup{
		matchGroup("keep", "keep.go", "keep.py"),
	}})
	if len(stale) != 2 || stale[0].ID != "aaaa" || stale[1].ID != "bbbb" {
		t.Fatalf("stale = %#v, want sorted aaaa/bbbb", stale)
	}
}

func TestLoadRejectsDuplicateIdentities(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "duplicate.json")
	writeFixture(t, path, `{"schema_version":2,"identity_scope":"content","normalization_version":6,"entries":[{"id":"same"},{"id":"same"}]}`)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "duplicate identity") {
		t.Fatalf("duplicate Load error = %v", err)
	}
}

func matchGroup(id string, leftPath string, rightPath string) model.MatchGroup {
	return model.MatchGroup{
		ID:         id,
		Similarity: 0.8,
		Profiles: []model.FragmentProfile{
			{
				Fingerprint:     "left",
				OccurrenceCount: 1,
				Occurrences:     []model.FragmentSummary{{Location: location(leftPath)}},
			},
			{
				Fingerprint:     "right",
				OccurrenceCount: 1,
				Occurrences:     []model.FragmentSummary{{Location: location(rightPath)}},
			},
		},
	}
}

func location(path string) model.Location {
	return model.Location{
		Path: path, Language: "go", LanguageFamily: "go", Name: "function", StartLine: 1, EndLine: 2,
	}
}

func writeFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if normalize.Version != 6 {
		t.Fatalf("test fixture version = %d, want 5", normalize.Version)
	}
}
