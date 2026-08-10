package analyzer

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

	if first.Files != 8 || first.Fragments != 8 {
		t.Fatalf("files/fragments = %d/%d, want 8/8", first.Files, first.Fragments)
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
		if group.Evidence.Union == 0 || math.Abs(
			group.Similarity-float64(group.Evidence.Intersection)/float64(group.Evidence.Union),
		) > 1e-12 {
			t.Fatalf("group has inconsistent structural evidence: %#v", group)
		}
		if group.Evidence.LeftOnly.Fingerprint != group.Profiles[0].Fingerprint {
			t.Fatalf("left evidence does not align with profile A: %#v", group)
		}
		if group.Evidence.LeftOnly.Features == nil || group.Evidence.RightOnly.Features == nil {
			t.Fatalf("directional evidence arrays must not be null: %#v", group.Evidence)
		}
		if len(group.Profiles) == 2 &&
			group.Evidence.RightOnly.Fingerprint != group.Profiles[1].Fingerprint {
			t.Fatalf("right evidence does not align with profile B: %#v", group)
		}
	}
}

func TestAnalyzePHPHackPositiveAndNearbyNegative(t *testing.T) {
	t.Parallel()

	positive := t.TempDir()
	writeAnalyzerFixture(t, positive, "validate.php", `<?php
function validate_email(string $value): bool {
  $clean = trim($value);
  if ($clean === '') return false;
  return str_contains($clean, '@') && str_contains($clean, '.');
}

`)
	writeAnalyzerFixture(t, positive, "check.hack", `function check_address(string $candidate): bool {
  $normalized = Str\trim($candidate);
  if ($normalized === '') return false;
  return Str\contains($normalized, '@') && Str\contains($normalized, '.');
}
`)
	positiveResult := analyzePHPHackFixture(t, positive, 0.70)
	if positiveResult.CandidatePairs != 1 || positiveResult.TotalMatchGroups != 1 ||
		len(positiveResult.Groups) != 1 || positiveResult.Groups[0].Similarity < 0.94 {
		t.Fatalf("PHP/Hack positive result = %#v", positiveResult)
	}

	negative := t.TempDir()
	writeAnalyzerFixture(t, negative, "validate.php", `<?php
function validate_email(string $value): bool {
  $clean = trim($value);
  return str_contains($clean, '@') && str_contains($clean, '.');
}
`)
	writeAnalyzerFixture(t, negative, "total.hack", `function total_values(vec<int> $values): int {
  $total = 0;
  foreach ($values as $value) $total += $value;
  return $total;
}
`)
	negativeResult := analyzePHPHackFixture(t, negative, 0.70)
	if negativeResult.CandidatePairs != 0 || negativeResult.TotalMatchGroups != 0 ||
		len(negativeResult.Groups) != 0 {
		t.Fatalf("PHP/Hack nearby-negative result = %#v", negativeResult)
	}
}

func TestAnalyzeCCPPPositiveAndNearbyNegative(t *testing.T) {
	t.Parallel()
	positive := t.TempDir()
	writeAnalyzerFixture(t, positive, "clamp.c", `int clamp_number(int value, int minimum, int maximum) {
  if (value < minimum) return minimum;
  if (value > maximum) return maximum;
  return value;
}
`)
	writeAnalyzerFixture(t, positive, "clamp.cpp", `int clampValue(int input, int lower, int upper) {
  if (input < lower) return lower;
  if (input > upper) return upper;
  return input;
}
`)
	positiveResult := analyzeTwoFileFixture(t, positive, Options{Threshold: 0.70, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10, MaxPairs: 10, Workers: 2, LanguagePairs: []LanguagePair{{Left: "c", Right: "cpp"}}})
	if positiveResult.CandidatePairs != 1 || positiveResult.TotalMatchGroups != 1 || len(positiveResult.Groups) != 1 || positiveResult.Groups[0].Similarity < 0.87 || positiveResult.Groups[0].Similarity > 0.89 {
		t.Fatalf("C/C++ positive result = %#v", positiveResult)
	}

	negative := t.TempDir()
	writeAnalyzerFixture(t, negative, "clamp.c", `int clamp_number(int value, int minimum, int maximum) {
  if (value < minimum) return minimum;
  if (value > maximum) return maximum;
  return value;
}
`)
	writeAnalyzerFixture(t, negative, "sum.cpp", `int sumValues(const int *values, int count) {
  int total = 0;
  for (int index = 0; index < count; index++) total += values[index];
  return total;
}
`)
	negativeResult := analyzeTwoFileFixture(t, negative, Options{Threshold: 0.70, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10, MaxPairs: 10, Workers: 2, LanguagePairs: []LanguagePair{{Left: "c", Right: "cpp"}}})
	if negativeResult.CandidatePairs != 1 || negativeResult.TotalMatchGroups != 0 {
		t.Fatalf("C/C++ nearby-negative result = %#v", negativeResult)
	}
}

func analyzePHPHackFixture(t *testing.T, root string, threshold float64) model.Report {
	t.Helper()
	return analyzeTwoFileFixture(t, root, Options{
		Threshold: threshold, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 2,
		LanguagePairs: []LanguagePair{{Left: "php", Right: "hack"}},
	})
}

func TestAnalyzeJavaCSharpPositiveAndNearbyNegative(t *testing.T) {
	t.Parallel()

	positive := t.TempDir()
	writeAnalyzerFixture(t, positive, "Eligibility.java", `
final class Eligibility {
  static boolean mayPublish(Document document) {
    if (document == null) return false;
    if (!approved(document)) return false;
    return containsPublicTag(document) && activeAuthor(document);
  }
}
`)
	writeAnalyzerFixture(t, positive, "Eligibility.cs", `
static class Eligibility {
  static bool MayPublish(Document document) {
    if (document == null) return false;
    if (!Approved(document)) return false;
    return ContainsPublicTag(document) && ActiveAuthor(document);
  }
}
`)
	positiveResult := analyzeLanguageExpansionFixture(t, positive, 0.70)
	if positiveResult.CandidatePairs != 1 || positiveResult.TotalMatchGroups != 1 ||
		len(positiveResult.Groups) != 1 || positiveResult.Groups[0].Similarity < 0.88 ||
		positiveResult.Groups[0].Similarity > 0.91 {
		t.Fatalf("Java/C# positive result = %#v", positiveResult)
	}

	negative := t.TempDir()
	writeAnalyzerFixture(t, negative, "Search.java", `
final class Search {
  static int binarySearch(int[] values, int target) {
    int low = 0;
    int high = values.length - 1;
    while (low <= high) {
      int middle = (low + high) / 2;
      if (values[middle] == target) return middle;
      if (values[middle] < target) low = middle + 1;
      else high = middle - 1;
    }
    return -1;
  }
}
`)
	writeAnalyzerFixture(t, negative, "Receipt.cs", `
static class Receipt {
  static string Render(Order order) {
    if (order == null) return "";
    var customer = Normalize(order.Customer.DisplayName);
    var address = Normalize(order.Customer.Address);
    var reference = FormatReference(order.Number);
    try {
      return string.Concat(customer, address, reference);
    } catch (FormatException) {
      return reference;
    }
  }
}
`)
	negativeResult := analyzeLanguageExpansionFixture(t, negative, 0.70)
	if negativeResult.CandidatePairs != 1 || negativeResult.TotalMatchGroups != 0 ||
		len(negativeResult.Groups) != 0 {
		t.Fatalf("Java/C# nearby-negative result = %#v", negativeResult)
	}
}

func writeAnalyzerFixture(t *testing.T, root string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func analyzeLanguageExpansionFixture(t *testing.T, root string, threshold float64) model.Report {
	t.Helper()
	return analyzeTwoFileFixture(t, root, Options{
		Threshold: threshold, MinTokens: 12, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 2, CrossLanguageOnly: true,
	})
}

func analyzeTwoFileFixture(t *testing.T, root string, options Options) model.Report {
	t.Helper()
	discovered := source.Discover([]string{root}, source.Options{MaxFileBytes: 1024 * 1024})
	if len(discovered.Warnings) != 0 || len(discovered.Files) != 2 {
		t.Fatalf("discovery = %#v", discovered)
	}
	result, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return result
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
		t.Fatalf("cross-family result = %+v", crossFamily)
	}

	if err := os.WriteFile(
		filepath.Join(root, "other.go"),
		[]byte("package sample\nfunc check(value string) string { return value }\n"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	discovered = source.Discover([]string{root}, source.Options{})

	sameFamily, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold: 1, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 1, SameLanguageOnly: true,
	})
	if err != nil {
		t.Fatalf("Analyze same-family: %v", err)
	}
	if sameFamily.CandidatePairs != 1 || sameFamily.TotalMatchGroups != 1 ||
		sameFamily.TotalLocationPairs != 1 {
		t.Fatalf("same-family result = %+v", sameFamily)
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

func TestAnalyzeNeverComparesShellScriptsWithFunctions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "script.zsh")
	content := "print_value() { print -r -- \"$1\"; }\nprint -r -- \"$1\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	discovered := source.Discover([]string{root}, source.Options{})
	result, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold: 0.5, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 1, SameLanguageOnly: true,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Fragments != 2 || result.CandidatePairs != 0 || result.TotalMatchGroups != 0 {
		t.Fatalf("script/function partition result = %+v", result)
	}

	if err := os.WriteFile(
		filepath.Join(root, "script.sh"),
		[]byte("printf '%s\\n' \"$1\"\n"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	discovered = source.Discover([]string{root}, source.Options{})
	explicit, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold: 0.5, MinTokens: 1, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 1,
		LanguagePairs: []LanguagePair{{Left: "bash", Right: "zsh"}},
	})
	if err != nil {
		t.Fatalf("Analyze explicit pair: %v", err)
	}
	if explicit.CandidatePairs != 1 {
		t.Fatalf("explicit script/function partition result = %+v", explicit)
	}
	for _, group := range explicit.Groups {
		for _, pair := range group.PathPairs {
			if pair.Left.FragmentKind != pair.Right.FragmentKind {
				t.Fatalf("cross-kind explicit match = %#v", pair)
			}
		}
	}
}

func TestAnalyzeRejectsConflictingLanguageSelectionModes(t *testing.T) {
	t.Parallel()

	_, err := Analyze(context.Background(), nil, nil, Options{
		Threshold: 1, MinTokens: 1, Workers: 1,
		SameLanguageOnly: true, CrossLanguageOnly: true,
	})
	if err == nil {
		t.Fatal("Analyze accepted conflicting language selection modes")
	}
}

func TestReviewPriorityUsesOnlyExplainableLocationSignals(t *testing.T) {
	t.Parallel()

	candidate := &groupCandidate{
		locationPairs: 2,
		pathPairs: map[string]model.LocationPair{
			"pair": {
				Left:  model.Location{Path: "internal/config/config.go", Name: "trackerKind"},
				Right: model.Location{Path: "internal/secrets/secrets.go", Name: "trackerKind"},
			},
		},
	}
	priority, signals := reviewPriority(candidate, nil)
	if priority != 10 {
		t.Fatalf("priority = %d, want 10", priority)
	}
	want := []string{
		"same-name-cross-directory",
		"cross-directory",
		"same-name-cross-file",
		"cross-file",
		"repeated-location-pairs",
	}
	if !reflect.DeepEqual(signals, want) {
		t.Fatalf("signals = %#v, want %#v", signals, want)
	}
}

func TestReviewPriorityAddsConfiguredPathRulesOnce(t *testing.T) {
	t.Parallel()

	candidate := &groupCandidate{
		locationPairs: 2,
		pathPairs: map[string]model.LocationPair{
			"one": {
				Left:  model.Location{Path: "internal/auth/file.go", Name: "authorize"},
				Right: model.Location{Path: "internal/store/file.go", Name: "authorize"},
			},
			"two": {
				Left:  model.Location{Path: "internal/auth/other.go", Name: "authorize"},
				Right: model.Location{Path: "internal/store/other.go", Name: "authorize"},
			},
		},
	}
	priority, signals := reviewPriority(candidate, []model.PriorityPathRule{{
		Pattern: "**/auth/**", Priority: 25,
	}})
	if priority != 35 {
		t.Fatalf("priority = %d, want base 10 plus configured 25", priority)
	}
	if signals[len(signals)-1] != "priority-path:**/auth/**(+25)" {
		t.Fatalf("signals = %#v", signals)
	}
}

func TestReviewRankingPrecedesStructuralScoreWhenRequested(t *testing.T) {
	t.Parallel()

	highReview := &groupCandidate{
		similarity: 0.9,
		id:         "high-review",
		left:       model.Fragment{FeatureCount: 10, Features: model.FeatureBag{}},
		right:      model.Fragment{FeatureCount: 10, Features: model.FeatureBag{}},
		profiles:   map[string]*profileAggregate{},
		pathPairs: map[string]model.LocationPair{
			"pair": {
				Left:  model.Location{Path: "config/a.go", Name: "kind"},
				Right: model.Location{Path: "secrets/b.go", Name: "kind"},
			},
		},
		focused: map[string]struct{}{},
	}
	structural := &groupCandidate{
		similarity: 1,
		id:         "structural",
		left:       model.Fragment{FeatureCount: 20, Features: model.FeatureBag{}},
		right:      model.Fragment{FeatureCount: 20, Features: model.FeatureBag{}},
		profiles:   map[string]*profileAggregate{},
		pathPairs: map[string]model.LocationPair{
			"pair": {
				Left:  model.Location{Path: "store.go", Name: "first"},
				Right: model.Location{Path: "store.go", Name: "second"},
			},
		},
		focused: map[string]struct{}{},
	}
	report := model.Report{}
	collector := matchCollector{
		options: Options{Ranking: RankingReview},
		report:  &report,
		groups: map[string]*groupCandidate{
			highReview.id: highReview,
			structural.id: structural,
		},
		suppressedGroups: map[string]struct{}{},
	}
	collector.finish()
	if len(report.Groups) != 2 || report.Groups[0].ID != highReview.id ||
		report.Groups[0].ReviewPriority <= report.Groups[1].ReviewPriority {
		t.Fatalf("review-ranked groups = %#v", report.Groups)
	}
}

func TestAnalyzeReportsEmptyCoverage(t *testing.T) {
	t.Parallel()

	options := Options{
		Threshold: 1, MinTokens: 12, MaxGroups: 10, MaxOccurrences: 10,
		MaxPairs: 10, Workers: 1,
	}
	empty, err := Analyze(context.Background(), nil, nil, options)
	if err != nil {
		t.Fatalf("Analyze empty: %v", err)
	}
	if len(empty.Warnings) != 1 || empty.Warnings[0].Kind != "coverage" ||
		!strings.Contains(empty.Warnings[0].Message, "no supported source files") {
		t.Fatalf("empty warnings = %#v", empty.Warnings)
	}
	if empty.Coverage.WarningCount != 1 || empty.Coverage.AnalyzedFiles != 0 {
		t.Fatalf("empty coverage = %#v", empty.Coverage)
	}

	root := t.TempDir()
	writeFile := filepath.Join(root, "declarations.go")
	if err := os.WriteFile(writeFile, []byte("package sample\nconst value = 1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	discovered := source.Discover([]string{root}, source.Options{})
	noFragments, err := Analyze(context.Background(), discovered.Files, nil, options)
	if err != nil {
		t.Fatalf("Analyze no fragments: %v", err)
	}
	if noFragments.Files != 1 || noFragments.Fragments != 0 || len(noFragments.Warnings) != 1 ||
		noFragments.Warnings[0].Kind != "coverage" ||
		!strings.Contains(noFragments.Warnings[0].Message, "no comparison fragments") {
		t.Fatalf("no-fragment report = %#v", noFragments)
	}
	if len(noFragments.FileCoverage) != 1 ||
		noFragments.FileCoverage[0].Path != discovered.Files[0].DisplayPath ||
		noFragments.FileCoverage[0].Language != "go" ||
		noFragments.FileCoverage[0].FragmentCount != 0 ||
		noFragments.FileCoverage[0].ZeroReason != "no_boundaries" {
		t.Fatalf("file coverage = %#v, want one zero-fragment Go file", noFragments.FileCoverage)
	}
	if noFragments.Coverage.SupportedFiles != 1 || noFragments.Coverage.AnalyzedFiles != 1 ||
		noFragments.Coverage.ZeroFragmentFiles != 1 || noFragments.Coverage.WarningCount != 1 {
		t.Fatalf("coverage summary = %#v", noFragments.Coverage)
	}
}

func TestAnalyzeExplainsBoundariesBelowTokenFloor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "small.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Small() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	discovered := source.Discover([]string{root}, source.Options{})
	result, err := Analyze(context.Background(), discovered.Files, nil, Options{
		Threshold:      0.7,
		MinTokens:      100,
		MaxGroups:      10,
		MaxOccurrences: 10,
		MaxPairs:       100,
		Workers:        1,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.FileCoverage) != 1 ||
		result.FileCoverage[0].CandidateCount != 1 ||
		result.FileCoverage[0].BelowTokenFloor != 1 ||
		result.FileCoverage[0].ZeroReason != "below_token_floor" {
		t.Fatalf("file coverage = %#v", result.FileCoverage)
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

func TestCollectorReportsSourceFreeLiteralDrift(t *testing.T) {
	t.Parallel()

	report := model.Report{}
	collector := matchCollector{
		ctx: context.Background(),
		options: Options{
			Threshold: 1, MaxGroups: 10, MaxOccurrences: 10, MaxPairs: 10,
		},
		report:           &report,
		groups:           make(map[string]*groupCandidate),
		suppressedGroups: make(map[string]struct{}),
	}
	left := groupingFragment("left.go", 1)
	left.LiteralDigests = []string{"same", "jpeg"}
	right := groupingFragment("right.go", 1)
	right.LiteralDigests = []string{"same", "avif", "fallback"}
	if err := collector.score(left, right); err != nil {
		t.Fatalf("score: %v", err)
	}
	collector.finish()

	if len(report.Groups) != 1 || report.Groups[0].LiteralEvidence == nil {
		t.Fatalf("groups = %#v, want literal evidence", report.Groups)
	}
	evidence := report.Groups[0].LiteralEvidence
	if evidence.ComparedPairs != 1 || evidence.PairsWithDifferences != 1 ||
		evidence.MaxDifferingPositions != 2 || evidence.LiteralCountMismatchPairs != 1 {
		t.Fatalf("literal evidence = %#v", evidence)
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

func TestCollectorSkipsOverlappingStatementBlockPairs(t *testing.T) {
	t.Parallel()

	report := model.Report{}
	collector := matchCollector{
		ctx:     context.Background(),
		options: Options{Threshold: 1, MaxGroups: 10, MaxOccurrences: 10, MaxPairs: 10},
		report:  &report,
		groups:  make(map[string]*groupCandidate), suppressedGroups: make(map[string]struct{}),
	}
	left := groupingFragment("same.go", 1)
	left.Location.FragmentKind = "block"
	left.StartByte, left.EndByte = 10, 40
	overlap := groupingFragment("same.go", 2)
	overlap.Location.FragmentKind = "block"
	overlap.StartByte, overlap.EndByte = 30, 60
	distinct := groupingFragment("same.go", 10)
	distinct.Location.FragmentKind = "block"
	distinct.StartByte, distinct.EndByte = 70, 100
	for _, pair := range [][2]model.Fragment{{left, overlap}, {left, distinct}, {overlap, distinct}} {
		if err := collector.score(pair[0], pair[1]); err != nil {
			t.Fatalf("score: %v", err)
		}
	}
	collector.finish()
	if report.CandidatePairs != 2 || report.TotalLocationPairs != 2 ||
		len(report.Groups) != 1 || report.Groups[0].LocationPairs != 2 {
		t.Fatalf("report = %+v, want overlapping pair omitted before candidate count", report)
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
