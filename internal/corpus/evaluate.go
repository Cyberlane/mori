// Package corpus evaluates Mori's redistributable labeled calibration corpus.
package corpus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/parser"
	"github.com/Cyberlane/mori/internal/similarity"
	"github.com/Cyberlane/mori/internal/source"
)

const SchemaVersion = 1

const (
	AcceptedPositive      = "accepted-positive"
	IntentionalSimilarity = "intentional-similarity"
	FalsePositive         = "false-positive"
)

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	License       string    `json:"license"`
	Reference     Reference `json:"reference"`
	Cases         []Case    `json:"cases"`
}

type Reference struct {
	MoriVersion          string `json:"mori_version"`
	NormalizationVersion int    `json:"normalization_version"`
}

type Case struct {
	ID             string    `json:"id"`
	Classification string    `json:"classification"`
	Reviewer       string    `json:"reviewer"`
	LanguagePair   [2]string `json:"language_pair"`
	Threshold      float64   `json:"threshold"`
	Expected       Expected  `json:"expected"`
	Left           Endpoint  `json:"left"`
	Right          Endpoint  `json:"right"`
}

type Expected struct {
	MinScore       float64  `json:"min_score"`
	MaxScore       float64  `json:"max_score"`
	Rank           int      `json:"rank"`
	ReferenceScore *float64 `json:"reference_score,omitempty"`
	ReferenceRank  *int     `json:"reference_rank,omitempty"`
}

type Endpoint struct {
	Path       string `json:"path"`
	Name       string `json:"name,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	SQLDialect string `json:"sql_dialect,omitempty"`
}

type Report struct {
	SchemaVersion        int       `json:"schema_version"`
	NormalizationVersion int       `json:"normalization_version"`
	Reference            Reference `json:"reference"`
	CaseCount            int       `json:"case_count"`
	Metrics              Metrics   `json:"metrics"`
	Cases                []Result  `json:"cases"`
	Violations           []string  `json:"violations"`
}

type Metrics struct {
	ReviewRelevantGroups int            `json:"review_relevant_distinct_groups"`
	ActionableGroups     int            `json:"actionable_distinct_groups"`
	FalsePositiveGroups  int            `json:"false_positive_distinct_groups"`
	Actionability        float64        `json:"actionability_by_distinct_group"`
	PrecisionAtK         []Precision    `json:"precision_at_k"`
	ScoreDistributions   []Distribution `json:"score_distributions"`
}

type Precision struct {
	K         int     `json:"k"`
	Relevant  int     `json:"review_relevant"`
	Precision float64 `json:"precision"`
}

type Distribution struct {
	Classification string  `json:"classification"`
	Count          int     `json:"count"`
	Minimum        float64 `json:"minimum"`
	Maximum        float64 `json:"maximum"`
	Mean           float64 `json:"mean"`
}

type Result struct {
	ID             string    `json:"id"`
	Classification string    `json:"classification"`
	Reviewer       string    `json:"reviewer"`
	LanguagePair   [2]string `json:"language_pair"`
	Threshold      float64   `json:"threshold"`
	Score          float64   `json:"score"`
	Rank           int       `json:"rank"`
	AboveThreshold bool      `json:"above_threshold"`
	FragmentTokens [2]int    `json:"fragment_tokens"`
	ReferenceScore *float64  `json:"reference_score,omitempty"`
	ScoreMovement  *float64  `json:"score_movement,omitempty"`
	ReferenceRank  *int      `json:"reference_rank,omitempty"`
	RankMovement   *int      `json:"rank_movement,omitempty"`
	Left           Endpoint  `json:"left"`
	Right          Endpoint  `json:"right"`
}

// Evaluate loads, validates, and scores a labeled corpus manifest. Expected
// score or rank drift is returned as deterministic report violations.
func Evaluate(ctx context.Context, root, manifestPath string) (Report, error) {
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return Report{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return Report{}, err
	}

	type parsedFile struct {
		language  string
		fragments []model.Fragment
	}
	cache := make(map[string]parsedFile)
	loadEndpoint := func(endpoint Endpoint) (model.Fragment, string, error) {
		clean, err := corpusPath(root, endpoint.Path)
		if err != nil {
			return model.Fragment{}, "", err
		}
		key := clean + "\x00" + endpoint.SQLDialect
		parsed, ok := cache[key]
		if !ok {
			var spec language.Spec
			if endpoint.SQLDialect != "" {
				var detected bool
				spec, detected = language.DetectWithSQLDialect(clean, endpoint.SQLDialect)
				if !detected {
					return model.Fragment{}, "", fmt.Errorf("detect %s with SQL dialect %s", endpoint.Path, endpoint.SQLDialect)
				}
			} else {
				var detected bool
				spec, detected = language.Detect(clean)
				if !detected {
					return model.Fragment{}, "", fmt.Errorf("detect %s", endpoint.Path)
				}
			}
			fragments, warnings := parser.File(ctx, source.File{
				Path: clean, DisplayPath: endpoint.Path, Language: spec,
			}, 1)
			if len(warnings) != 0 {
				return model.Fragment{}, "", fmt.Errorf("parse %s: %s", endpoint.Path, warnings[0].Message)
			}
			parsed = parsedFile{language: spec.ID, fragments: fragments}
			cache[key] = parsed
		}
		matches := make([]model.Fragment, 0, 1)
		for _, fragment := range parsed.fragments {
			if endpoint.Name != "" && fragment.Location.Name != endpoint.Name {
				continue
			}
			if endpoint.StartLine > 0 && fragment.Location.StartLine != endpoint.StartLine {
				continue
			}
			matches = append(matches, fragment)
		}
		if len(matches) != 1 {
			return model.Fragment{}, "", fmt.Errorf("select %s: matched %d fragments, want 1", endpoint.Path, len(matches))
		}
		return matches[0], parsed.language, nil
	}

	results := make([]Result, 0, len(manifest.Cases))
	for _, corpusCase := range manifest.Cases {
		left, leftLanguage, err := loadEndpoint(corpusCase.Left)
		if err != nil {
			return Report{}, fmt.Errorf("case %s left: %w", corpusCase.ID, err)
		}
		right, rightLanguage, err := loadEndpoint(corpusCase.Right)
		if err != nil {
			return Report{}, fmt.Errorf("case %s right: %w", corpusCase.ID, err)
		}
		if corpusCase.LanguagePair != [2]string{leftLanguage, rightLanguage} {
			return Report{}, fmt.Errorf(
				"case %s language pair = %v, detected %s,%s",
				corpusCase.ID, corpusCase.LanguagePair, leftLanguage, rightLanguage,
			)
		}
		score, _, _ := similarity.WeightedJaccard(left.Features, right.Features)
		result := Result{
			ID: corpusCase.ID, Classification: corpusCase.Classification,
			Reviewer: corpusCase.Reviewer, LanguagePair: corpusCase.LanguagePair,
			Threshold: corpusCase.Threshold, Score: score,
			AboveThreshold: score >= corpusCase.Threshold,
			FragmentTokens: [2]int{left.TokenCount, right.TokenCount},
			ReferenceScore: corpusCase.Expected.ReferenceScore,
			ReferenceRank:  corpusCase.Expected.ReferenceRank,
			Left:           corpusCase.Left, Right: corpusCase.Right,
		}
		if result.ReferenceScore != nil {
			movement := score - *result.ReferenceScore
			result.ScoreMovement = &movement
		}
		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})
	expected := make(map[string]Expected, len(manifest.Cases))
	for _, corpusCase := range manifest.Cases {
		expected[corpusCase.ID] = corpusCase.Expected
	}
	violations := make([]string, 0)
	for index := range results {
		results[index].Rank = index + 1
		want := expected[results[index].ID]
		if results[index].Score < want.MinScore || results[index].Score > want.MaxScore {
			violations = append(violations, fmt.Sprintf(
				"%s score %.6f outside [%.6f, %.6f]",
				results[index].ID, results[index].Score, want.MinScore, want.MaxScore,
			))
		}
		if results[index].Rank != want.Rank {
			violations = append(violations, fmt.Sprintf(
				"%s rank %d does not match %d", results[index].ID, results[index].Rank, want.Rank,
			))
		}
		if results[index].ReferenceRank != nil {
			movement := results[index].Rank - *results[index].ReferenceRank
			results[index].RankMovement = &movement
		}
	}

	return Report{
		SchemaVersion: SchemaVersion, NormalizationVersion: normalize.Version,
		Reference: manifest.Reference, CaseCount: len(results),
		Metrics: buildMetrics(results), Cases: results,
		Violations: violations,
	}, nil
}

func loadManifest(path string) (Manifest, error) {
	opened, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer opened.Close()
	decoder := json.NewDecoder(opened)
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode corpus manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New("decode corpus manifest: trailing JSON value")
		}
		return Manifest{}, fmt.Errorf("decode corpus manifest: %w", err)
	}
	return manifest, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("corpus schema version %d is unsupported; expected %d", manifest.SchemaVersion, SchemaVersion)
	}
	if manifest.License == "" || manifest.Reference.MoriVersion == "" ||
		manifest.Reference.NormalizationVersion <= 0 || len(manifest.Cases) == 0 {
		return errors.New("corpus manifest requires a license and at least one case")
	}
	seen := make(map[string]struct{}, len(manifest.Cases))
	for _, corpusCase := range manifest.Cases {
		if corpusCase.ID == "" || corpusCase.Reviewer == "" {
			return errors.New("every corpus case requires an id and reviewer")
		}
		if _, exists := seen[corpusCase.ID]; exists {
			return fmt.Errorf("duplicate corpus case id %q", corpusCase.ID)
		}
		seen[corpusCase.ID] = struct{}{}
		switch corpusCase.Classification {
		case AcceptedPositive, IntentionalSimilarity, FalsePositive:
		default:
			return fmt.Errorf("case %s has unsupported classification %q", corpusCase.ID, corpusCase.Classification)
		}
		if corpusCase.Threshold <= 0 || corpusCase.Threshold > 1 ||
			corpusCase.Expected.MinScore < 0 || corpusCase.Expected.MaxScore > 1 ||
			corpusCase.Expected.MinScore > corpusCase.Expected.MaxScore || corpusCase.Expected.Rank <= 0 {
			return fmt.Errorf("case %s has invalid threshold, score range, or rank", corpusCase.ID)
		}
		if corpusCase.Expected.ReferenceScore != nil &&
			(*corpusCase.Expected.ReferenceScore < 0 || *corpusCase.Expected.ReferenceScore > 1) {
			return fmt.Errorf("case %s has an invalid reference score", corpusCase.ID)
		}
		if corpusCase.Expected.ReferenceRank != nil && *corpusCase.Expected.ReferenceRank <= 0 {
			return fmt.Errorf("case %s has an invalid reference rank", corpusCase.ID)
		}
		if corpusCase.LanguagePair[0] == "" || corpusCase.LanguagePair[1] == "" {
			return fmt.Errorf("case %s requires two declared languages", corpusCase.ID)
		}
		for _, endpoint := range []Endpoint{corpusCase.Left, corpusCase.Right} {
			if endpoint.Path == "" || (endpoint.Name == "" && endpoint.StartLine <= 0) {
				return fmt.Errorf("case %s endpoint requires a path and fragment selector", corpusCase.ID)
			}
			if _, err := corpusPath(".", endpoint.Path); err != nil {
				return fmt.Errorf("case %s: %w", corpusCase.ID, err)
			}
		}
	}
	return nil
}

func corpusPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("corpus path %q must be relative", relative)
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("corpus path %q escapes the corpus root", relative)
	}
	return filepath.Join(root, clean), nil
}

func buildMetrics(results []Result) Metrics {
	metrics := Metrics{}
	for _, result := range results {
		switch result.Classification {
		case AcceptedPositive:
			metrics.ReviewRelevantGroups++
			metrics.ActionableGroups++
		case IntentionalSimilarity:
			metrics.ReviewRelevantGroups++
		case FalsePositive:
			metrics.FalsePositiveGroups++
		}
	}
	if len(results) > 0 {
		metrics.Actionability = float64(metrics.ActionableGroups) / float64(len(results))
	}
	for _, requested := range []int{1, 3, 5, 10} {
		if requested > len(results) {
			continue
		}
		relevant := 0
		for _, result := range results[:requested] {
			if result.Classification != FalsePositive {
				relevant++
			}
		}
		metrics.PrecisionAtK = append(metrics.PrecisionAtK, Precision{
			K: requested, Relevant: relevant, Precision: float64(relevant) / float64(requested),
		})
	}
	for _, classification := range []string{AcceptedPositive, IntentionalSimilarity, FalsePositive} {
		distribution := Distribution{Classification: classification}
		total := 0.0
		for _, result := range results {
			if result.Classification != classification {
				continue
			}
			if distribution.Count == 0 || result.Score < distribution.Minimum {
				distribution.Minimum = result.Score
			}
			if distribution.Count == 0 || result.Score > distribution.Maximum {
				distribution.Maximum = result.Score
			}
			distribution.Count++
			total += result.Score
		}
		if distribution.Count > 0 {
			distribution.Mean = total / float64(distribution.Count)
			metrics.ScoreDistributions = append(metrics.ScoreDistributions, distribution)
		}
	}
	return metrics
}
