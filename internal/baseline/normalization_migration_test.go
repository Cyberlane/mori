package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
)

func TestExplicitNormalizationMigrationRetainsDecisions(t *testing.T) {
	t.Parallel()
	if normalize.Version != 13 {
		t.Fatal("review explicit migration support when normalization advances")
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	profile := testProfile(0.7)
	entries := []Entry{{ID: "matched", Similarity: 0.9, Classification: "intentional", Note: "retained decision", Left: model.Location{Path: "left.go"}, Right: model.Location{Path: "right.go"}}, {ID: "stale", Similarity: 0.8, Classification: "false-positive", Note: "reassess later"}}
	if err := writeEntries(path, entries, profile.Threshold, ScopeContent, profile); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc.NormalizationVersion = 12
	old, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("ordinary load accepted previous normalization")
	}
	if _, err := Decode(old); err == nil {
		t.Fatal("staged decode accepted previous normalization")
	}
	set, err := LoadForMigration(path)
	if err != nil {
		t.Fatal(err)
	}
	report := model.Report{Groups: []model.MatchGroup{{ID: "matched", PathPairs: []model.LocationPair{{Left: entries[0].Left, Right: entries[0].Right}}}}}
	stale := Stale(set, report)
	if len(stale) != 1 || stale[0].ID != "stale" {
		t.Fatalf("stale decisions: %+v", stale)
	}
	if err := Migrate(path, set, profile); err != nil {
		t.Fatal(err)
	}
	migrated, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(set.Entries(), migrated.Entries()) {
		t.Fatal("migration changed decisions or metadata")
	}
	for _, version := range []int{11, 14, 999} {
		doc.NormalizationVersion = version
		raw, err = json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadForMigration(path); err == nil {
			t.Fatalf("migration accepted normalization %d", version)
		}
	}
}
