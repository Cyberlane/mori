package similarity

import (
	"math"
	"reflect"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestWeightedJaccard(t *testing.T) {
	t.Parallel()

	left := model.FeatureBag{"function": 1, "return": 1, "call": 2}
	right := model.FeatureBag{"function": 1, "return": 1, "call": 1, "branch": 1}

	score, intersection, union := WeightedJaccard(left, right)
	if intersection != 3 {
		t.Fatalf("intersection = %d, want 3", intersection)
	}
	if union != 5 {
		t.Fatalf("union = %d, want 5", union)
	}
	if math.Abs(score-0.6) > 1e-9 {
		t.Fatalf("score = %f, want 0.6", score)
	}
}

func TestWeightedJaccardEmpty(t *testing.T) {
	t.Parallel()

	score, intersection, union := WeightedJaccard(nil, nil)
	if score != 0 || intersection != 0 || union != 0 {
		t.Fatalf("empty score = %f, %d, %d; want zeroes", score, intersection, union)
	}
}

func TestSharedIsDeterministic(t *testing.T) {
	t.Parallel()

	left := model.FeatureBag{"z": 2, "a": 2, "m": 1}
	right := model.FeatureBag{"z": 2, "a": 2, "m": 4}
	want := []model.SharedFeature{
		{Feature: "a", Count: 2},
		{Feature: "z", Count: 2},
	}
	if got := Shared(left, right, 2); !reflect.DeepEqual(got, want) {
		t.Fatalf("Shared() = %#v, want %#v", got, want)
	}
}

func TestEvidenceReportsExactTotalsAndBoundedDirectionalDifferences(t *testing.T) {
	t.Parallel()

	left := model.FeatureBag{"shared": 2, "left-large": 4, "left-tie": 1}
	right := model.FeatureBag{"shared": 3, "right-large": 5, "right-tie": 1}
	evidence := Evidence("b", right, "a", left, 1)
	if evidence.Intersection != 2 || evidence.Union != 14 {
		t.Fatalf("weighted totals = %d/%d, want 2/14", evidence.Intersection, evidence.Union)
	}
	if evidence.LeftOnly.Fingerprint != "a" || evidence.LeftOnly.Total != 5 ||
		!reflect.DeepEqual(evidence.LeftOnly.Features, []model.SharedFeature{{Feature: "left-large", Count: 4}}) {
		t.Fatalf("left-only evidence = %#v", evidence.LeftOnly)
	}
	if evidence.RightOnly.Fingerprint != "b" || evidence.RightOnly.Total != 7 ||
		!reflect.DeepEqual(evidence.RightOnly.Features, []model.SharedFeature{{Feature: "right-large", Count: 5}}) {
		t.Fatalf("right-only evidence = %#v", evidence.RightOnly)
	}
}

func TestShapeSummarizesCanonicalStructureWithoutNames(t *testing.T) {
	t.Parallel()

	left := model.FeatureBag{
		"node:expression:call": 3,
		"node:flow:if":         2,
		"node:flow:return":     1,
	}
	right := model.FeatureBag{
		"node:expression:call": 4,
		"node:flow:if":         1,
		"node:flow:return":     1,
	}
	shape := Shape(left, right)
	want := []string{"3 calls", "1 conditional branch", "1 return"}
	if !reflect.DeepEqual(shape, want) {
		t.Fatalf("Shape = %#v, want %#v", shape, want)
	}
}
