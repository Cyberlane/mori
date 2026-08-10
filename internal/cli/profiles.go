package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Cyberlane/mori/internal/analyzer"
	"github.com/Cyberlane/mori/internal/config"
)

const (
	profileReview  = "review"
	profileExplore = "explore"
	profileSQL     = "sql"
)

type scanProfile struct {
	name              string
	threshold         float64
	minTokens         int
	maxGroups         int
	maxOccurrences    int
	comparisonDomain  string
	ranking           string
	sameLanguageOnly  bool
	crossLanguageOnly bool
	requireCoverage   bool
	minFileCoverage   float64
	maxZeroFiles      int
	failOnWarning     bool
	failOnDiagnostic  bool
	excludeGenerated  bool
}

func resolveScanProfile(value string) (scanProfile, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	profiles := map[string]scanProfile{
		profileReview: {
			name:             profileReview,
			threshold:        0.85,
			minTokens:        40,
			maxGroups:        250,
			maxOccurrences:   10,
			comparisonDomain: "code",
			ranking:          analyzer.RankingReview,
			sameLanguageOnly: true,
			requireCoverage:  true,
			excludeGenerated: true,
			maxZeroFiles:     -1,
		},
		profileExplore: {
			name:           profileExplore,
			threshold:      0.70,
			minTokens:      12,
			maxGroups:      100,
			maxOccurrences: 20,
			ranking:        analyzer.RankingStructural,
			maxZeroFiles:   -1,
		},
		profileSQL: {
			name:             profileSQL,
			threshold:        0.70,
			minTokens:        12,
			maxGroups:        250,
			maxOccurrences:   10,
			comparisonDomain: "sql-query",
			ranking:          analyzer.RankingReview,
			requireCoverage:  true,
			excludeGenerated: true,
			maxZeroFiles:     -1,
		},
	}
	profile, ok := profiles[value]
	if !ok {
		return scanProfile{}, fmt.Errorf(
			"unknown profile %q; expected review, explore, or sql",
			value,
		)
	}
	return profile, nil
}

func applyScanProfile(options *scanOptions, value string) error {
	profile, err := resolveScanProfile(value)
	if err != nil {
		return err
	}
	options.profile = profile.name
	options.threshold = profile.threshold
	options.minTokens = profile.minTokens
	options.maxGroups = profile.maxGroups
	options.maxOccurrences = profile.maxOccurrences
	options.comparisonDomain = profile.comparisonDomain
	options.ranking = profile.ranking
	options.sameLanguageOnly = profile.sameLanguageOnly
	options.crossLanguageOnly = profile.crossLanguageOnly
	options.requireCoverage = profile.requireCoverage
	options.minFileCoverage = profile.minFileCoverage
	options.maxZeroFiles = profile.maxZeroFiles
	options.failOnWarning = profile.failOnWarning
	options.failOnDiagnostic = profile.failOnDiagnostic
	options.excludeGenerated = profile.excludeGenerated
	return nil
}

func renderProfileConfig(value string) ([]byte, error) {
	profile, err := resolveScanProfile(value)
	if err != nil {
		return nil, err
	}
	settings := config.Settings{
		Profile:           profile.name,
		Threshold:         pointer(profile.threshold),
		MinTokens:         pointer(profile.minTokens),
		MaxGroups:         pointer(profile.maxGroups),
		MaxOccurrences:    pointer(profile.maxOccurrences),
		ComparisonDomain:  profile.comparisonDomain,
		Ranking:           profile.ranking,
		SameLanguageOnly:  pointer(profile.sameLanguageOnly),
		CrossLanguageOnly: pointer(profile.crossLanguageOnly),
		RequireCoverage:   pointer(profile.requireCoverage),
		MinFileCoverage:   pointer(profile.minFileCoverage),
		MaxZeroFiles:      pointer(profile.maxZeroFiles),
		FailOnWarning:     pointer(profile.failOnWarning),
		FailOnDiagnostic:  pointer(profile.failOnDiagnostic),
		ExcludeGenerated:  pointer(profile.excludeGenerated),
	}
	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return append(content, '\n'), nil
}

func pointer[T any](value T) *T {
	return &value
}
