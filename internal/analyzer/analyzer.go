// Package analyzer orchestrates parsing, comparison, and deterministic results.
package analyzer

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/Cyberlane/mori/internal/fingerprint"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/parser"
	"github.com/Cyberlane/mori/internal/similarity"
	"github.com/Cyberlane/mori/internal/source"
)

// Options controls parsing concurrency and pair selection.
type Options struct {
	Threshold         float64
	MinTokens         int
	MaxMatches        int
	MaxPairs          int
	Workers           int
	CrossLanguageOnly bool
}

type parseJob struct {
	index int
	file  source.File
}

type parseResult struct {
	index     int
	fragments []model.Fragment
	warnings  []model.Warning
}

type matchCandidate struct {
	similarity float64
	id         string
	left       model.Fragment
	right      model.Fragment
}

type candidateHeap []matchCandidate

func (h candidateHeap) Len() int { return len(h) }

func (h candidateHeap) Less(i int, j int) bool {
	return candidateBetter(h[j], h[i])
}

func (h candidateHeap) Swap(i int, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *candidateHeap) Push(value any) {
	*h = append(*h, value.(matchCandidate))
}

func (h *candidateHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	old[last] = matchCandidate{}
	*h = old[:last]
	return value
}

type matchCollector struct {
	ctx       context.Context
	options   Options
	report    *model.Report
	bounded   candidateHeap
	unbounded []matchCandidate
}

// Analyze parses files and returns every pair at or above the threshold.
func Analyze(
	ctx context.Context,
	files []source.File,
	initialWarnings []model.Warning,
	options Options,
) (model.Report, error) {
	report := model.Report{
		SchemaVersion: model.SchemaVersion,
		Threshold:     options.Threshold,
		Files:         len(files),
		Matches:       make([]model.Match, 0),
		Warnings:      append(make([]model.Warning, 0, len(initialWarnings)), initialWarnings...),
	}
	if err := validateOptions(options); err != nil {
		return report, err
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if len(files) == 0 {
		return report, nil
	}

	workers := options.Workers
	if workers < 1 {
		workers = 1
	}
	if workers > len(files) {
		workers = len(files)
	}

	jobs := make(chan parseJob)
	results := make(chan parseResult, len(files))
	var workerGroup sync.WaitGroup
	workerGroup.Add(workers)

	for range workers {
		go func() {
			defer workerGroup.Done()
			for job := range jobs {
				if ctx.Err() != nil {
					return
				}
				fragments, warnings := parser.File(ctx, job.file, options.MinTokens)
				results <- parseResult{
					index:     job.index,
					fragments: fragments,
					warnings:  warnings,
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for index, file := range files {
			select {
			case <-ctx.Done():
				return
			case jobs <- parseJob{index: index, file: file}:
			}
		}
	}()

	go func() {
		workerGroup.Wait()
		close(results)
	}()

	parsed := make([]parseResult, len(files))
	for result := range results {
		parsed[result.index] = result
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}

	fragments := make([]model.Fragment, 0)
	for _, result := range parsed {
		fragments = append(fragments, result.fragments...)
		report.Warnings = append(report.Warnings, result.warnings...)
	}
	sortFragments(fragments)
	sortWarnings(report.Warnings)
	report.Fragments = len(fragments)

	ordered := append([]model.Fragment(nil), fragments...)
	sort.Slice(ordered, func(i, j int) bool {
		return fragmentLess(ordered[i], ordered[j])
	})

	collector := matchCollector{
		ctx:     ctx,
		options: options,
		report:  &report,
	}
	var compareErr error
	if options.CrossLanguageOnly {
		compareErr = compareAcrossLanguages(ordered, &collector)
	} else {
		compareErr = compareAll(ordered, &collector)
	}
	if compareErr != nil {
		return report, compareErr
	}
	collector.finish()

	return report, nil
}

func validateOptions(options Options) error {
	if math.IsNaN(options.Threshold) || math.IsInf(options.Threshold, 0) ||
		options.Threshold <= 0 || options.Threshold > 1 {
		return errors.New("threshold must be finite, greater than 0, and at most 1")
	}
	if options.MinTokens < 1 {
		return errors.New("minimum token count must be at least 1")
	}
	if options.MaxMatches < 0 || options.MaxPairs < 0 {
		return errors.New("maximum values cannot be negative")
	}
	return nil
}

func compareAll(fragments []model.Fragment, collector *matchCollector) error {
	for leftIndex := 0; leftIndex < len(fragments); leftIndex++ {
		if err := collector.ctx.Err(); err != nil {
			return err
		}
		left := fragments[leftIndex]
		if left.FeatureCount == 0 {
			continue
		}
		for rightIndex := leftIndex + 1; rightIndex < len(fragments); rightIndex++ {
			right := fragments[rightIndex]
			if right.FeatureCount == 0 {
				continue
			}
			if sizeUpperBound(left.FeatureCount, right.FeatureCount) < collector.options.Threshold {
				break
			}
			if err := collector.score(left, right); err != nil {
				return err
			}
		}
	}
	return nil
}

func compareAcrossLanguages(fragments []model.Fragment, collector *matchCollector) error {
	byLanguage := make(map[string][]model.Fragment)
	for _, fragment := range fragments {
		byLanguage[fragment.Location.Language] = append(
			byLanguage[fragment.Location.Language],
			fragment,
		)
	}
	languages := make([]string, 0, len(byLanguage))
	for languageID := range byLanguage {
		languages = append(languages, languageID)
	}
	sort.Strings(languages)

	for leftLanguage := 0; leftLanguage < len(languages); leftLanguage++ {
		for rightLanguage := leftLanguage + 1; rightLanguage < len(languages); rightLanguage++ {
			if err := compareLanguagePair(
				byLanguage[languages[leftLanguage]],
				byLanguage[languages[rightLanguage]],
				collector,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func compareLanguagePair(
	leftFragments []model.Fragment,
	rightFragments []model.Fragment,
	collector *matchCollector,
) error {
	for _, left := range leftFragments {
		if err := collector.ctx.Err(); err != nil {
			return err
		}
		if left.FeatureCount == 0 {
			continue
		}

		firstPossible := sort.Search(len(rightFragments), func(index int) bool {
			return float64(rightFragments[index].FeatureCount)/float64(left.FeatureCount) >=
				collector.options.Threshold
		})
		for _, right := range rightFragments[firstPossible:] {
			if right.FeatureCount == 0 {
				continue
			}
			upperBound := sizeUpperBound(left.FeatureCount, right.FeatureCount)
			if upperBound < collector.options.Threshold {
				if right.FeatureCount >= left.FeatureCount {
					break
				}
				continue
			}
			if fragmentLess(right, left) {
				if err := collector.score(right, left); err != nil {
					return err
				}
			} else if err := collector.score(left, right); err != nil {
				return err
			}
		}
	}
	return nil
}

func (collector *matchCollector) score(left model.Fragment, right model.Fragment) error {
	if err := collector.ctx.Err(); err != nil {
		return err
	}
	if collector.options.MaxPairs > 0 &&
		collector.report.CandidatePairs >= collector.options.MaxPairs {
		return fmt.Errorf(
			"candidate pair limit of %d reached; narrow the scan, raise --min-tokens, "+
				"raise --threshold, or increase --max-pairs",
			collector.options.MaxPairs,
		)
	}

	collector.report.CandidatePairs++
	score, _, _ := similarity.WeightedJaccard(left.Features, right.Features)
	if score < collector.options.Threshold {
		return nil
	}

	collector.report.TotalMatches++
	candidate := matchCandidate{similarity: score, left: left, right: right}
	candidate.id = fingerprint.Pair(left.Fingerprint, right.Fingerprint)
	if collector.options.MaxMatches == 0 {
		collector.unbounded = append(collector.unbounded, candidate)
		return nil
	}
	if len(collector.bounded) < collector.options.MaxMatches {
		heap.Push(&collector.bounded, candidate)
		return nil
	}
	if candidateBetter(candidate, collector.bounded[0]) {
		collector.bounded[0] = candidate
		heap.Fix(&collector.bounded, 0)
	}
	return nil
}

func (collector *matchCollector) finish() {
	candidates := collector.unbounded
	if collector.options.MaxMatches > 0 {
		candidates = append([]matchCandidate(nil), collector.bounded...)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidateBetter(candidates[i], candidates[j])
	})

	collector.report.Matches = make([]model.Match, 0, len(candidates))
	for _, candidate := range candidates {
		collector.report.Matches = append(collector.report.Matches, model.Match{
			ID:             candidate.id,
			Similarity:     candidate.similarity,
			Left:           candidate.left.Summary(),
			Right:          candidate.right.Summary(),
			SharedFeatures: similarity.Shared(candidate.left.Features, candidate.right.Features, 8),
		})
	}
	collector.report.Truncated = collector.report.TotalMatches > len(collector.report.Matches)
}

func candidateBetter(left matchCandidate, right matchCandidate) bool {
	if left.similarity != right.similarity {
		return left.similarity > right.similarity
	}
	leftKey := locationKey(left.left.Location)
	rightKey := locationKey(right.left.Location)
	if leftKey != rightKey {
		return leftKey < rightKey
	}
	return locationKey(left.right.Location) < locationKey(right.right.Location)
}

func fragmentLess(left model.Fragment, right model.Fragment) bool {
	if left.FeatureCount != right.FeatureCount {
		return left.FeatureCount < right.FeatureCount
	}
	return locationKey(left.Location) < locationKey(right.Location)
}

func sizeUpperBound(leftCount int, rightCount int) float64 {
	if leftCount <= 0 || rightCount <= 0 {
		return 0
	}
	smaller := min(leftCount, rightCount)
	larger := max(leftCount, rightCount)
	return float64(smaller) / float64(larger)
}

func sortFragments(fragments []model.Fragment) {
	sort.Slice(fragments, func(i, j int) bool {
		return locationKey(fragments[i].Location) < locationKey(fragments[j].Location)
	})
}

func sortWarnings(warnings []model.Warning) {
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Path == warnings[j].Path {
			return warnings[i].Message < warnings[j].Message
		}
		return warnings[i].Path < warnings[j].Path
	})
}

func locationKey(location model.Location) string {
	return fmt.Sprintf(
		"%s:%09d:%09d:%s:%s",
		location.Path,
		location.StartLine,
		location.EndLine,
		location.Language,
		location.Name,
	)
}
