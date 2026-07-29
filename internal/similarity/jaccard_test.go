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
