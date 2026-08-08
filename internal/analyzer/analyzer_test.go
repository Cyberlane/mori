package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/source"
)

func TestAnalyzeCrossLanguageExamples(t *testing.T) {
	t.Parallel()

	discovered := source.Discover(
		[]string{"../../examples/email-validation"},
		source.Options{MaxFileBytes: 1024 * 1024},
	)
	if len(discovered.Warnings) != 0 {
		t.Fatalf("discovery warnings = %#v", discovered.Warnings)
	}

	options := Options{
		Threshold:         0.70,
		MinTokens:         12,
		MaxGroups:         100,
		MaxOccurrences:    20,
		MaxPairs:          100,
		Workers:           2,
		CrossLanguageOnly: true,
	}
	first, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	second, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil {
		t.Fatalf("Analyze second run: %v", err)
	}

	if first.Files != 4 || first.Fragments != 4 {
		t.Fatalf("files/fragments = %d/%d, want 4/4", first.Files, first.Fragments)
	}
	if len(first.Groups) < 2 {
		t.Fatalf("groups = %d, want at least 2", len(first.Groups))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("analysis is not deterministic across identical runs")
	}

	top := first.Groups[0]
	if len(top.Profiles) == 0 || len(top.Profiles[0].Occurrences) == 0 {
		t.Fatalf("top group has no occurrences: %#v", top)
	}
	if top.Similarity < options.Threshold {
		t.Fatalf("top score = %f, below threshold", top.Similarity)
	}
	for _, group := range first.Groups {
		if group.ID == "" {
			t.Fatal("group has no stable ID")
		}
		for _, profile := range group.Profiles {
			if profile.Fingerprint == "" {
				t.Fatalf("group has incomplete fragment identities: %#v", group)
			}
		}
	}
}

func TestCrossLanguageUsesFamiliesAndExplicitPairsUseGrammarIDs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, name := range []string{"left.ts", "right.tsx"} {
		content := "export const check = (value: string) => { return value.trim(); };\n"
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	discovered := source.Discover([]string{root}, source.Options{})
	crossFamily, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold: 1, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 1, CrossLanguageOnly: true,
	})
	if err != nil {
		t.Fatalf("Analyze cross-family: %v", err)
	}
	if crossFamily.CandidatePairs != 0 || crossFamily.TotalMatchGroups != 0 {
		t.Fatalf("TS/TSX appeared cross-family: %+v", crossFamily)
	}

	explicit, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold: 1, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 1,
		LanguagePairs: []LanguagePair{{Left: "typescript", Right: "tsx"}},
	})
	if err != nil {
		t.Fatalf("Analyze explicit pair: %v", err)
	}
	if explicit.TotalMatchGroups != 1 || explicit.TotalLocationPairs != 1 {
		t.Fatalf("explicit TS/TSX result = %+v", explicit)
	}
}

func TestAnalyzeBoundsReportedMatches(t *testing.T) {
	t.Parallel()

	discovered := source.Discover(
		[]string{"../../examples/email-validation"},
		source.Options{MaxFileBytes: 1024 * 1024},
	)
	result, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold:         0.70,
		MinTokens:         12,
		MaxGroups:         1,
		MaxOccurrences:    20,
		MaxPairs:          100,
		Workers:           2,
		CrossLanguageOnly: true,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.TotalMatchGroups < 2 {
		t.Fatalf("total groups = %d, want at least 2", result.TotalMatchGroups)
	}
	if len(result.Groups) != 1 || !result.Truncated {
		t.Fatalf(
			"reported matches/truncated = %d/%t, want 1/true",
			len(result.Groups),
			result.Truncated,
		)
	}
}

func TestAnalyzeHonorsPairLimit(t *testing.T) {
	t.Parallel()

	discovered := source.Discover(
		[]string{"../../examples/email-validation"},
		source.Options{MaxFileBytes: 1024 * 1024},
	)
	_, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold: 0.10,
		MinTokens: 1,
		MaxGroups: 100,
		MaxPairs:  1,
		Workers:   1,
	})
	if err == nil {
		t.Fatal("Analyze returned nil after exceeding pair limit")
	}
}

func TestCompatibleDomainsNeverCompareSQLWithCode(t *testing.T) {
	t.Parallel()

	report := model.Report{}
	collector := matchCollector{
		ctx:              context.Background(),
		options:          Options{Threshold: 1, MaxPairs: 10},
		report:           &report,
		groups:           make(map[string]*groupCandidate),
		suppressedGroups: make(map[string]struct{}),
	}
	code := groupingFragment("code.go", 1)
	code.Location.ComparisonDomain = "code"
	sql := groupingFragment("query.sql", 1)
	sql.Location.Language = "sql"
	sql.Location.LanguageFamily = "sql"
	sql.Location.ComparisonDomain = "sql-query"
	if err := compareCompatibleDomains([]model.Fragment{code, sql}, &collector); err != nil {
		t.Fatalf("compareCompatibleDomains: %v", err)
	}
	if report.CandidatePairs != 0 || len(collector.groups) != 0 {
		t.Fatalf("cross-domain comparison state = %+v/%d groups", report, len(collector.groups))
	}
}

func TestCollectorSuppressesBeforeRetention(t *testing.T) {
	t.Parallel()

	report := model.Report{}
	collector := matchCollector{
		ctx: context.Background(),
		options: Options{
			Threshold: 0.5,
			MaxGroups: 1,
			MaxPairs:  10,
			Suppress: func(id string, _ model.Location, _ model.Location) bool {
				return id == "left:right"
			},
		},
		report:           &report,
		groups:           make(map[string]*groupCandidate),
		suppressedGroups: make(map[string]struct{}),
	}
	left := model.Fragment{
		Fingerprint:  "left",
		FeatureCount: 1,
		Features:     model.FeatureBag{"node:return": 1},
	}
	right := model.Fragment{
		Fingerprint:  "right",
		FeatureCount: 1,
		Features:     model.FeatureBag{"node:return": 1},
	}
	if err := collector.score(left, right); err != nil {
		t.Fatalf("score suppressed: %v", err)
	}
	if report.SuppressedLocationPairs != 1 || report.TotalLocationPairs != 0 || len(collector.groups) != 0 {
		t.Fatalf("suppressed state = %+v, groups = %d", report, len(collector.groups))
	}

	right.Fingerprint = "other"
	if err := collector.score(left, right); err != nil {
		t.Fatalf("score accepted: %v", err)
	}
	if report.SuppressedLocationPairs != 1 || report.TotalLocationPairs != 1 || len(collector.groups) != 1 {
		t.Fatalf("accepted state = %+v, groups = %d", report, len(collector.groups))
	}
}

func TestCollectorSuppressesOnUnboundedPath(t *testing.T) {
	t.Parallel()

	report := model.Report{}
	collector := matchCollector{
		ctx: context.Background(),
		options: Options{
			Threshold: 0.5,
			MaxPairs:  10,
			Suppress: func(string, model.Location, model.Location) bool {
				return true
			},
		},
		report:           &report,
		groups:           make(map[string]*groupCandidate),
		suppressedGroups: make(map[string]struct{}),
	}
	fragment := model.Fragment{
		Fingerprint:  "same",
		FeatureCount: 1,
		Features:     model.FeatureBag{"node:return": 1},
	}
	if err := collector.score(fragment, fragment); err != nil {
		t.Fatalf("score: %v", err)
	}
	if report.SuppressedLocationPairs != 1 || report.TotalLocationPairs != 0 || len(collector.groups) != 0 {
		t.Fatalf("state = %+v, groups = %d", report, len(collector.groups))
	}
}

func TestCollectorGroupsEquivalentLocationPairs(t *testing.T) {
	t.Parallel()

	report := model.Report{}
	collector := matchCollector{
		ctx: context.Background(),
		options: Options{
			Threshold: 0.5, MaxGroups: 10, MaxOccurrences: 10, MaxPairs: 10,
		},
		report:           &report,
		groups:           make(map[string]*groupCandidate),
		suppressedGroups: make(map[string]struct{}),
	}
	fragments := []model.Fragment{
		groupingFragment("a.go", 1),
		groupingFragment("b.go", 10),
		groupingFragment("c.go", 20),
	}
	for left := 0; left < len(fragments); left++ {
		for right := left + 1; right < len(fragments); right++ {
			if err := collector.score(fragments[left], fragments[right]); err != nil {
				t.Fatalf("score: %v", err)
			}
		}
	}
	collector.finish()
	if report.TotalLocationPairs != 3 || report.TotalMatchGroups != 1 || len(report.Groups) != 1 {
		t.Fatalf("grouped report = %+v", report)
	}
	group := report.Groups[0]
	if group.LocationPairs != 3 || len(group.Profiles) != 1 ||
		group.Profiles[0].OccurrenceCount != 3 || len(group.Profiles[0].Occurrences) != 3 {
		t.Fatalf("group = %+v", group)
	}
}

func TestCollectorPrioritizesFocusedGroupsBeforeRetention(t *testing.T) {
	t.Parallel()

	report := model.Report{}
	collector := matchCollector{
		ctx: context.Background(),
		options: Options{
			Threshold: 0.5, MaxGroups: 1, MaxOccurrences: 1, MaxPairs: 10,
			FocusActive: true,
			FocusPaths:  map[string]struct{}{"focused.go": {}},
		},
		report:           &report,
		groups:           make(map[string]*groupCandidate),
		suppressedGroups: make(map[string]struct{}),
	}

	nonFocusedLeft := groupingFragment("best-a.go", 1)
	nonFocusedLeft.Fingerprint = "best-a"
	nonFocusedRight := groupingFragment("best-b.go", 1)
	nonFocusedRight.Fingerprint = "best-b"
	if err := collector.score(nonFocusedLeft, nonFocusedRight); err != nil {
		t.Fatalf("score non-focused: %v", err)
	}

	focusedLeft := groupingFragment("focused.go", 1)
	focusedLeft.Fingerprint = "focused"
	focusedLeft.FeatureCount = 2
	focusedLeft.Features = model.FeatureBag{"node:flow:return": 1, "node:branch:if": 1}
	focusedRight := groupingFragment("existing.go", 1)
	focusedRight.Fingerprint = "existing"
	if err := collector.score(focusedLeft, focusedRight); err != nil {
		t.Fatalf("score focused: %v", err)
	}

	secondFocused := focusedLeft
	secondFocused.Location.StartLine = 10
	secondFocused.Location.EndLine = 11
	if err := collector.score(secondFocused, focusedRight); err != nil {
		t.Fatalf("score second focused occurrence: %v", err)
	}
	collector.finish()

	if report.TotalMatchGroups != 2 || report.TotalFocusedMatchGroups != 1 || len(report.Groups) != 1 {
		t.Fatalf("focused totals = %+v", report)
	}
	if !report.Groups[0].Focused || report.Groups[0].FocusedCount != 2 {
		t.Fatalf("retained group = %+v, want two exact focused occurrences", report.Groups[0])
	}
	if len(report.Groups[0].Profiles[0].Occurrences) != 1 {
		t.Fatalf("occurrence sample was not bounded: %+v", report.Groups[0].Profiles)
	}
}

func groupingFragment(path string, line int) model.Fragment {
	return model.Fragment{
		Location: model.Location{
			Path: path, Language: "go", LanguageFamily: "go", Name: "same",
			StartLine: line, EndLine: line + 1,
		},
		TokenCount: 1, FeatureCount: 1, Fingerprint: "same",
		Features: model.FeatureBag{"node:flow:return": 1},
	}
}
