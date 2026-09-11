package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/source"
)

func TestReviewShortlistRetainsSmallLogicAndDemotesRepeatedWrappers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 0; i < 6; i++ {
		dir := filepath.Join(root, fmt.Sprintf("package%d", i))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		// Nearby negatives: arithmetic and conditional logic must not receive
		// the wrapper penalty, despite identical names and repeated locations.
		code := `package fixture
func (store *StoreA) GetByID(id int) Item { return store.Lookup(id) }
func (store *StoreB) GetByID(id int) Item { return store.Lookup(id) }
func CalendarDateKey(utc, offset int) int { return (utc + offset) / 86400 }
func ValidateExecution(value int) bool { if value < 0 { return false }; return true }
`
		if err := os.WriteFile(filepath.Join(dir, "review.go"), []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
	}
	discovered := source.Discover([]string{root}, source.Options{MaxFileBytes: 1 << 20})
	options := Options{Threshold: 1, MinTokens: 1, Workers: 2, Ranking: RankingReview}
	review, err := Analyze(context.Background(), discovered.Files, discovered.Warnings, options)
	if err != nil || len(review.Warnings) != 0 {
		t.Fatalf("scan %v warnings %v", err, review.Warnings)
	}
	if len(review.Groups) != 3 {
		t.Fatalf("groups=%d", len(review.Groups))
	}
	penalty := "repeated-small-wrapper(-7)"
	for i, group := range review.Groups {
		name := group.Profiles[0].Occurrences[0].Location.Name
		if name == "GetByID" {
			if i != 2 || !slices.Contains(group.ReviewSignals, penalty) {
				t.Fatalf("wrapper rank/signals %d %v", i, group.ReviewSignals)
			}
		} else if slices.Contains(group.ReviewSignals, penalty) {
			t.Fatalf("useful small logic penalized: %s", name)
		}
	}
	options.Ranking = RankingStructural
	structural, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	identities := func(report model.Report) map[string]float64 {
		out := map[string]float64{}
		for _, group := range report.Groups {
			out[group.ID] = group.Similarity
		}
		return out
	}
	if !reflect.DeepEqual(identities(review), identities(structural)) || review.TotalLocationPairs != structural.TotalLocationPairs {
		t.Fatal("ranking changed findings")
	}
	// Path baselines can leave only within-file method pairs. They must not
	// acquire a negative priority by removing a boost they never received.
	options.Ranking = RankingReview
	options.Suppress = func(_ string, left, right model.Location) bool {
		return filepath.Dir(left.Path) != filepath.Dir(right.Path)
	}
	local, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil || len(local.Groups) != 1 {
		t.Fatalf("local pairs: %v %d groups", err, len(local.Groups))
	}
	if local.Groups[0].ReviewPriority < 0 || slices.Contains(local.Groups[0].ReviewSignals, penalty) {
		t.Fatal("local-only pairs received cross-directory penalty")
	}
}
