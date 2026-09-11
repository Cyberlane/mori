package analyzer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/parser"
	"github.com/Cyberlane/mori/internal/similarity"
	"github.com/Cyberlane/mori/internal/source"
)

// workloadFiles exercises discovery and the real Go grammar, normalization,
// candidate enumeration and occurrence grouping with repeated and near shapes.
func workloadFiles(t testing.TB, count int) []source.File {
	t.Helper()
	root := t.TempDir()
	var code strings.Builder
	code.WriteString("package workload\n")
	for i := 0; i < count; i++ {
		if i%3 == 0 {
			fmt.Fprintf(&code, "func f%d(x int) int { if x > 0 { return x + 1 }; return 0 }\n", i)
		} else if i%3 == 1 {
			fmt.Fprintf(&code, "func f%d(x int) int { if x > 0 { return x - 2 }; return 1 }\n", i)
		} else {
			fmt.Fprintf(&code, "func f%d(x int) int { total := 0; for j := 0; j < x; j++ { total += j }; return total }\n", i)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "workload.go"), []byte(code.String()), 0600); err != nil {
		t.Fatal(err)
	}
	discovered := source.Discover([]string{root}, source.Options{MaxFileBytes: 1 << 20})
	if len(discovered.Warnings) != 0 {
		t.Fatal(discovered.Warnings)
	}
	return discovered.Files
}

func TestWorkloadDeterminismLimitsAndCancellation(t *testing.T) {
	files := workloadFiles(t, 60)
	opts := Options{Threshold: .7, MinTokens: 1, Workers: 1}
	first, err := Analyze(context.Background(), files, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fragments != 60 || first.TotalLocationPairs == 0 {
		t.Fatalf("unexpected workload coverage: %+v", first)
	}
	opts.Workers = 4
	second, err := Analyze(context.Background(), files, nil, opts)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("worker determinism: %v", err)
	}
	opts.EstimateOnly = true
	opts.MaxPairs = 1
	estimate, err := Analyze(context.Background(), files, nil, opts)
	if err != nil || estimate.CandidatePairs != first.CandidatePairs || len(estimate.Groups) != 0 || estimate.TotalLocationPairs != 0 {
		t.Fatalf("estimate disagrees with full scan: %+v %v", estimate, err)
	}
	opts.EstimateOnly = false
	opts.MaxPairs = 10
	limited, err := Analyze(context.Background(), files, nil, opts)
	var limit *CandidateLimitError
	if !errors.As(err, &limit) || limited.CandidatePairs != 10 {
		t.Fatalf("limit accounting: %d %v", limited.CandidatePairs, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Analyze(ctx, files, nil, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func BenchmarkAnalyzeReviewWorkload(b *testing.B) {
	files := workloadFiles(b, 400)
	opts := Options{Threshold: .85, MinTokens: 1, Workers: 4, MaxGroups: 25, MaxOccurrences: 10}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := Analyze(context.Background(), files, nil, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func TestScoreCacheReferenceAndCollision(t *testing.T) {
	fragments := []model.Fragment{
		{Fingerprint: "a", Features: model.FeatureBag{"x": 4, "y": 2}, FeatureCount: 6},
		{Fingerprint: "a", Features: model.FeatureBag{"x": 4, "y": 2}, FeatureCount: 6},
		{Fingerprint: "b", Features: model.FeatureBag{"x": 3, "y": 2}, FeatureCount: 5},
		{Fingerprint: "b", Features: model.FeatureBag{"x": 3, "y": 2}, FeatureCount: 5},
		{Fingerprint: "collision", Features: model.FeatureBag{"x": 6}, FeatureCount: 6},
		{Fingerprint: "collision", Features: model.FeatureBag{"z": 6}, FeatureCount: 6},
	}
	collector := matchCollector{options: Options{Threshold: .7}}
	collector.prepareScoreCache(fragments)
	if collector.repeatedBags["collision"] {
		t.Fatal("collision accepted for score reuse")
	}
	for pass := 0; pass < 3; pass++ {
		for _, left := range fragments {
			for _, right := range fragments {
				want, _, _ := similarity.WeightedJaccard(left.Features, right.Features)
				if want < .7 {
					want = 0
				}
				if got := collector.cachedScore(left, right); got != want {
					t.Fatalf("cache changed score %v versus %v", got, want)
				}
			}
		}
	}
	if len(collector.scoreCache) != 3 {
		t.Fatalf("cached pairs %d want3", len(collector.scoreCache))
	}
}

// This test crosses the real parser and production Analyze entry point. Its
// reference enumerates every unordered pair without the optimized size-search
// loops and injects exact legacy scores into a collector, sharing only report
// aggregation. Thus score pruning/reuse and pair enumeration are independently
// checked while occurrence, literal and path evidence must remain byte-for-byte
// equal, including focused and selectively suppressed results.
func TestAnalyzeReferenceEnumeration(t *testing.T) {
	files := workloadFiles(t, 30)
	var fragments []model.Fragment
	for _, file := range files {
		parsed, warnings := parser.File(context.Background(), file, 1)
		if len(warnings) > 0 {
			t.Fatal(warnings)
		}
		fragments = append(fragments, parsed...)
	}
	sort.Slice(fragments, func(i, j int) bool { return fragmentLess(fragments[i], fragments[j]) })
	for _, threshold := range []float64{.01, .7, .85, 1} {
		for _, mode := range []string{"all", "focused", "suppressed", "limited"} {
			t.Run(fmt.Sprintf("%g/%s", threshold, mode), func(t *testing.T) {
				opts := Options{Threshold: threshold, MinTokens: 1, Workers: 3, MaxGroups: 2, MaxOccurrences: 3}
				if mode == "focused" {
					opts.FocusActive = true
					opts.FocusedOnly = true
					opts.FocusIntervals = map[string][]model.LineInterval{files[0].Path: {{StartLine: 3, EndLine: 8}}}
				}
				if mode == "suppressed" {
					opts.Suppress = func(_ string, left, right model.Location) bool {
						return left.StartLine%2 == 0 && right.StartLine%3 == 0
					}
				}
				if mode == "limited" {
					opts.MaxPairs = 11
				}
				actual, actualErr := Analyze(context.Background(), files, nil, opts)
				expected := model.Report{Groups: []model.MatchGroup{}}
				reference := matchCollector{ctx: context.Background(), options: opts, report: &expected, groups: map[string]*groupCandidate{}, suppressedGroups: map[string]struct{}{}, repeatedBags: map[string]bool{}, scoreCache: map[[2]string]float64{}}
				for _, left := range fragments {
					for _, right := range fragments {
						reference.repeatedBags[left.Fingerprint] = true
						key := [2]string{left.Fingerprint, right.Fingerprint}
						if key[1] < key[0] {
							key[0], key[1] = key[1], key[0]
						}
						score, _, _ := similarity.WeightedJaccard(left.Features, right.Features)
						reference.scoreCache[key] = score
					}
				}
				var referenceErr error
			outer:
				for i, left := range fragments {
					for _, right := range fragments[i+1:] {
						// Deliberately no sorting-dependent break or binary-search optimization.
						smaller, larger := min(left.FeatureCount, right.FeatureCount), max(left.FeatureCount, right.FeatureCount)
						if larger == 0 || float64(smaller)/float64(larger) < threshold {
							continue
						}
						if err := reference.score(left, right); err != nil {
							referenceErr = err
							break outer
						}
					}
				}
				if referenceErr == nil {
					reference.finish()
				}
				if fmt.Sprint(actualErr) != fmt.Sprint(referenceErr) {
					t.Fatalf("error mismatch %v versus %v", actualErr, referenceErr)
				}
				if actual.CandidatePairs != expected.CandidatePairs || actual.TotalLocationPairs != expected.TotalLocationPairs || actual.SuppressedLocationPairs != expected.SuppressedLocationPairs || actual.SuppressedMatchGroups != expected.SuppressedMatchGroups || actual.TotalMatchGroups != expected.TotalMatchGroups || actual.Truncated != expected.Truncated || !reflect.DeepEqual(actual.Groups, expected.Groups) {
					t.Fatalf("optimized result differs from reference: candidates %d/%d pairs %d/%d suppressed %d/%d groups %d/%d", actual.CandidatePairs, expected.CandidatePairs, actual.TotalLocationPairs, expected.TotalLocationPairs, actual.SuppressedLocationPairs, expected.SuppressedLocationPairs, actual.TotalMatchGroups, expected.TotalMatchGroups)
				}
			})
		}
	}
}
