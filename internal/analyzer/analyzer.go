// Package analyzer orchestrates parsing, comparison, and deterministic results.
package analyzer

import (
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
	MaxGroups         int
	MaxOccurrences    int
	MaxPairs          int
	Workers           int
	CrossLanguageOnly bool
	LanguagePairs     []LanguagePair
	Suppress          func(id string, left model.Location, right model.Location) bool
}

// LanguagePair selects one concrete grammar-ID pair for comparison.
type LanguagePair struct {
	Left  string
	Right string
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

type profileAggregate struct {
	fingerprint  string
	tokenCount   int
	featureCount int
	occurrences  map[string]model.FragmentSummary
}

type groupCandidate struct {
	similarity    float64
	id            string
	left          model.Fragment
	right         model.Fragment
	locationPairs int
	profiles      map[string]*profileAggregate
	pathPairs     map[string]model.LocationPair
}

type matchCollector struct {
	ctx              context.Context
	options          Options
	report           *model.Report
	groups           map[string]*groupCandidate
	suppressedGroups map[string]struct{}
}

// Analyze parses files and returns content-grouped pairs at or above the
// threshold.
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
		Groups:        make([]model.MatchGroup, 0),
		Warnings:      append(make([]model.Warning, 0, len(initialWarnings)), initialWarnings...),
		Configuration: model.EffectiveConfig{
			IgnoreFiles:   make([]string, 0),
			Excludes:      make([]string, 0),
			LanguagePairs: make([]string, 0),
		},
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
		ctx:              ctx,
		options:          options,
		report:           &report,
		groups:           make(map[string]*groupCandidate),
		suppressedGroups: make(map[string]struct{}),
	}
	var compareErr error
	if len(options.LanguagePairs) > 0 {
		compareErr = compareSelectedLanguagePairs(ordered, options.LanguagePairs, &collector)
	} else if options.CrossLanguageOnly {
		compareErr = compareAcrossFamilies(ordered, &collector)
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
	if options.MaxGroups < 0 || options.MaxOccurrences < 0 || options.MaxPairs < 0 {
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

func compareAcrossFamilies(fragments []model.Fragment, collector *matchCollector) error {
	byFamily := make(map[string][]model.Fragment)
	for _, fragment := range fragments {
		byFamily[fragment.Location.LanguageFamily] = append(
			byFamily[fragment.Location.LanguageFamily],
			fragment,
		)
	}
	families := make([]string, 0, len(byFamily))
	for family := range byFamily {
		families = append(families, family)
	}
	sort.Strings(families)

	for leftFamily := 0; leftFamily < len(families); leftFamily++ {
		for rightFamily := leftFamily + 1; rightFamily < len(families); rightFamily++ {
			if err := compareLanguagePair(
				byFamily[families[leftFamily]],
				byFamily[families[rightFamily]],
				collector,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func compareSelectedLanguagePairs(
	fragments []model.Fragment,
	pairs []LanguagePair,
	collector *matchCollector,
) error {
	byLanguage := make(map[string][]model.Fragment)
	for _, fragment := range fragments {
		byLanguage[fragment.Location.Language] = append(
			byLanguage[fragment.Location.Language],
			fragment,
		)
	}
	seen := make(map[string]struct{})
	for _, pair := range pairs {
		leftID, rightID := pair.Left, pair.Right
		if rightID < leftID {
			leftID, rightID = rightID, leftID
		}
		key := leftID + "\x00" + rightID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if leftID == rightID {
			if err := compareAll(byLanguage[leftID], collector); err != nil {
				return err
			}
			continue
		}
		if err := compareLanguagePair(byLanguage[leftID], byLanguage[rightID], collector); err != nil {
			return err
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

	id := fingerprint.Pair(left.Fingerprint, right.Fingerprint)
	if collector.options.Suppress != nil && collector.options.Suppress(id, left.Location, right.Location) {
		collector.report.SuppressedLocationPairs++
		collector.suppressedGroups[id] = struct{}{}
		return nil
	}
	collector.report.TotalLocationPairs++
	group, exists := collector.groups[id]
	if !exists {
		group = &groupCandidate{
			similarity: score,
			id:         id,
			left:       left,
			right:      right,
			profiles:   make(map[string]*profileAggregate, 2),
			pathPairs:  make(map[string]model.LocationPair),
		}
		collector.groups[id] = group
	}
	group.locationPairs++
	group.addPathPair(left.Location, right.Location)
	if candidateBetter(left, right, group.left, group.right) {
		group.left = left
		group.right = right
	}
	collector.addProfileOccurrence(group, left)
	collector.addProfileOccurrence(group, right)
	return nil
}

func (collector *matchCollector) finish() {
	candidates := make([]*groupCandidate, 0, len(collector.groups))
	for _, candidate := range collector.groups {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return groupBetter(candidates[i], candidates[j])
	})
	collector.report.TotalMatchGroups = len(candidates)
	collector.report.SuppressedMatchGroups = len(collector.suppressedGroups)
	if collector.options.MaxGroups > 0 && len(candidates) > collector.options.MaxGroups {
		candidates = candidates[:collector.options.MaxGroups]
		collector.report.Truncated = true
	}

	collector.report.Groups = make([]model.MatchGroup, 0, len(candidates))
	for _, candidate := range candidates {
		collector.report.Groups = append(collector.report.Groups, model.MatchGroup{
			ID:             candidate.id,
			Similarity:     candidate.similarity,
			LocationPairs:  candidate.locationPairs,
			Profiles:       publicProfiles(candidate.profiles, collector.options.MaxOccurrences),
			ShapeSummary:   similarity.Shape(candidate.left.Features, candidate.right.Features),
			SharedFeatures: similarity.Shared(candidate.left.Features, candidate.right.Features, 8),
			PathPairs:      publicPathPairs(candidate.pathPairs),
		})
	}
}

func (group *groupCandidate) addPathPair(left model.Location, right model.Location) {
	leftKey := locationKey(left)
	rightKey := locationKey(right)
	if rightKey < leftKey {
		left, right = right, left
		leftKey, rightKey = rightKey, leftKey
	}
	group.pathPairs[leftKey+"\x00"+rightKey] = model.LocationPair{Left: left, Right: right}
}

func publicPathPairs(pairs map[string]model.LocationPair) []model.LocationPair {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]model.LocationPair, 0, len(keys))
	for _, key := range keys {
		result = append(result, pairs[key])
	}
	return result
}

func (collector *matchCollector) addProfileOccurrence(
	group *groupCandidate,
	fragment model.Fragment,
) {
	profile, exists := group.profiles[fragment.Fingerprint]
	if !exists {
		profile = &profileAggregate{
			fingerprint:  fragment.Fingerprint,
			tokenCount:   fragment.TokenCount,
			featureCount: fragment.FeatureCount,
			occurrences:  make(map[string]model.FragmentSummary),
		}
		group.profiles[fragment.Fingerprint] = profile
	}
	key := locationKey(fragment.Location)
	if _, exists := profile.occurrences[key]; exists {
		return
	}
	profile.occurrences[key] = fragment.Summary()
}

func publicProfiles(profiles map[string]*profileAggregate, limit int) []model.FragmentProfile {
	result := make([]model.FragmentProfile, 0, len(profiles))
	for _, profile := range profiles {
		occurrences := make([]model.FragmentSummary, 0, len(profile.occurrences))
		for _, occurrence := range profile.occurrences {
			occurrences = append(occurrences, occurrence)
		}
		sort.Slice(occurrences, func(i, j int) bool {
			return locationKey(occurrences[i].Location) < locationKey(occurrences[j].Location)
		})
		occurrenceCount := len(occurrences)
		if limit > 0 && len(occurrences) > limit {
			occurrences = occurrences[:limit]
		}
		result = append(result, model.FragmentProfile{
			Fingerprint:     profile.fingerprint,
			TokenCount:      profile.tokenCount,
			FeatureCount:    profile.featureCount,
			OccurrenceCount: occurrenceCount,
			Occurrences:     occurrences,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Fingerprint < result[j].Fingerprint })
	return result
}

func groupBetter(left *groupCandidate, right *groupCandidate) bool {
	if left.similarity != right.similarity {
		return left.similarity > right.similarity
	}
	leftEvidence := min(left.left.FeatureCount, left.right.FeatureCount)
	rightEvidence := min(right.left.FeatureCount, right.right.FeatureCount)
	if leftEvidence != rightEvidence {
		return leftEvidence > rightEvidence
	}
	if left.locationPairs != right.locationPairs {
		return left.locationPairs > right.locationPairs
	}
	return left.id < right.id
}

func candidateBetter(
	leftA model.Fragment,
	rightA model.Fragment,
	leftB model.Fragment,
	rightB model.Fragment,
) bool {
	leftKey := locationKey(leftA.Location)
	rightKey := locationKey(leftB.Location)
	if leftKey != rightKey {
		return leftKey < rightKey
	}
	return locationKey(rightA.Location) < locationKey(rightB.Location)
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
