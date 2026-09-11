package similarity

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

// referenceJaccard deliberately enumerates the union, independently of the
// production traversal. Keep this oracle simple when optimizing the engine.
func referenceJaccard(a, b model.FeatureBag) (float64, int, int) {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	intersection, union := 0, 0
	for k := range keys {
		intersection += min(a[k], b[k])
		union += max(a[k], b[k])
	}
	if union == 0 {
		return 0, 0, 0
	}
	return float64(intersection) / float64(union), intersection, union
}

func TestScoringExhaustiveReference(t *testing.T) {
	bags := make([]model.FeatureBag, 256)
	for n := range bags {
		bags[n] = model.FeatureBag{}
		for f := 0; f < 4; f++ {
			if count := (n >> (2 * f)) & 3; count > 0 {
				bags[n][fmt.Sprint(f)] = count
			}
		}
	}
	for _, a := range bags {
		for _, b := range bags {
			want, wi, wu := referenceJaccard(a, b)
			got, gi, gu := WeightedJaccard(a, b)
			if got != want || gi != wi || gu != wu {
				t.Fatalf("%v/%v got %v %d %d want %v %d %d", a, b, got, gi, gu, want, wi, wu)
			}
			at, bt := 0, 0
			for _, count := range a {
				at += count
			}
			for _, count := range b {
				bt += count
			}
			for _, threshold := range []float64{.01, .5, .7, .85, 1, want, math.Nextafter(want, 1), math.Nextafter(want, 0)} {
				actual := AtLeast(a, b, at, bt, threshold)
				expected := want
				if want < threshold {
					expected = 0
				}
				if actual != expected {
					t.Fatalf("threshold %v bags %v/%v got %v want %v", threshold, a, b, actual, expected)
				}
			}
		}
	}
}

func BenchmarkScoringWorkloads(b *testing.B) {
	for _, kind := range []string{"identical", "unrelated", "near"} {
		b.Run(kind, func(b *testing.B) {
			a, other := model.FeatureBag{}, model.FeatureBag{}
			for n := 0; n < 128; n++ {
				a[fmt.Sprint(n)] = n%5 + 1
				key := n
				if kind == "unrelated" {
					key += 128
				}
				other[fmt.Sprint(key)] = n%5 + 1
			}
			if kind == "near" {
				other["0"] = 2
			}
			at, bt := 0, 0
			for _, count := range a {
				at += count
			}
			for _, count := range other {
				bt += count
			}
			b.Run("reference", func(b *testing.B) {
				b.ReportAllocs()
				for n := 0; n < b.N; n++ {
					WeightedJaccard(a, other)
				}
			})
			b.Run("prepared", func(b *testing.B) {
				b.ReportAllocs()
				for n := 0; n < b.N; n++ {
					AtLeast(a, other, at, bt, .85)
				}
			})
		})
	}
}

// Wide bags exercise early rejection, unlike the small exhaustive state space.
func TestWideScoringReference(t *testing.T) {
	random := rand.New(rand.NewSource(20260911))
	for n := 0; n < 2000; n++ {
		a, b := model.FeatureBag{}, model.FeatureBag{}
		at, bt := 0, 0
		for f := 0; f < 128; f++ {
			ac, bc := random.Intn(9), random.Intn(9)
			key := fmt.Sprint(f)
			if ac > 0 {
				a[key] = ac
				at += ac
			}
			if bc > 0 {
				b[key] = bc
				bt += bc
			}
		}
		want, _, _ := referenceJaccard(a, b)
		for _, threshold := range []float64{.1, .5, .7, .85, 1, want, math.Nextafter(want, 1), math.Nextafter(want, 0)} {
			got := AtLeast(a, b, at, bt, threshold)
			expected := want
			if want < threshold {
				expected = 0
			}
			if got != expected {
				t.Fatalf("iteration%d threshold%.17g got%.17g want%.17g", n, threshold, got, expected)
			}
		}
	}
}
