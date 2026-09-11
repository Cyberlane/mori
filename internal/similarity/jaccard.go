// Package similarity scores normalized AST feature bags.
package similarity

import (
	"fmt"
	"sort"

	"github.com/Cyberlane/mori/internal/model"
)

// WeightedJaccard returns multiset Jaccard similarity, its intersection size,
// and its union size.
func WeightedJaccard(left model.FeatureBag, right model.FeatureBag) (float64, int, int) {
	intersection := 0
	union := 0

	for feature, leftCount := range left {
		rightCount := right[feature]
		intersection += min(leftCount, rightCount)
		union += max(leftCount, rightCount)
	}
	for feature, rightCount := range right {
		if _, exists := left[feature]; !exists {
			union += rightCount
		}
	}

	if union == 0 {
		return 0, 0, 0
	}
	return float64(intersection) / float64(union), intersection, union
}

// AtLeast scores positive feature multisets using their precomputed totals.
// Rejected pairs return zero. Accepted pairs retain exactly the same division
// as WeightedJaccard. The upper bound uses integer weights and the original
// floating-point comparison, avoiding rounded threshold transformations.
func AtLeast(left, right model.FeatureBag, leftTotal, rightTotal int, threshold float64) float64 {
	if len(left) > len(right) {
		left, right = right, left
		leftTotal, rightTotal = rightTotal, leftTotal
	}
	intersection, remaining, visited := 0, leftTotal, 0
	for feature, count := range left {
		intersection += min(count, right[feature])
		remaining -= count
		visited++
		if visited%8 == 0 {
			upper := min(intersection+remaining, rightTotal)
			union := leftTotal + rightTotal - upper
			if union > 0 && float64(upper)/float64(union) < threshold {
				return 0
			}
		}
	}
	union := leftTotal + rightTotal - intersection
	if union == 0 {
		return 0
	}
	score := float64(intersection) / float64(union)
	if score < threshold {
		return 0
	}
	return score
}

// Shape returns a compact, non-semantic summary of shared canonical structure.
func Shape(left model.FeatureBag, right model.FeatureBag) []string {
	type shapeFeature struct {
		feature  string
		singular string
		plural   string
	}
	features := []shapeFeature{
		{feature: "node:expression:call", singular: "call", plural: "calls"},
		{feature: "node:flow:if", singular: "conditional branch", plural: "conditional branches"},
		{feature: "node:flow:loop", singular: "loop", plural: "loops"},
		{feature: "node:flow:switch", singular: "switch or match", plural: "switches or matches"},
		{feature: "node:flow:return", singular: "return", plural: "returns"},
		{feature: "node:binding", singular: "binding", plural: "bindings"},
	}
	result := make([]string, 0, len(features))
	for _, feature := range features {
		count := min(left[feature.feature], right[feature.feature])
		if count == 0 {
			continue
		}
		label := feature.plural
		if count == 1 {
			label = feature.singular
		}
		result = append(result, fmt.Sprintf("%d %s", count, label))
	}
	return result
}

// Shared returns the most influential shared features in deterministic order.
func Shared(left model.FeatureBag, right model.FeatureBag, limit int) []model.SharedFeature {
	shared := make([]model.SharedFeature, 0)
	for feature, leftCount := range left {
		count := min(leftCount, right[feature])
		if count > 0 {
			shared = append(shared, model.SharedFeature{Feature: feature, Count: count})
		}
	}

	sort.Slice(shared, func(i, j int) bool {
		if shared[i].Count == shared[j].Count {
			return shared[i].Feature < shared[j].Feature
		}
		return shared[i].Count > shared[j].Count
	})
	if limit > 0 && len(shared) > limit {
		shared = shared[:limit]
	}
	return shared
}

// Evidence returns exact weighted totals plus bounded directional differences.
// Sides are ordered by fingerprint so they align with report profile ordering.
func Evidence(
	leftFingerprint string,
	left model.FeatureBag,
	rightFingerprint string,
	right model.FeatureBag,
	limit int,
) model.StructuralEvidence {
	if rightFingerprint < leftFingerprint {
		leftFingerprint, rightFingerprint = rightFingerprint, leftFingerprint
		left, right = right, left
	}
	_, intersection, union := WeightedJaccard(left, right)
	leftTotal, leftOnly := directionalDifference(left, right, limit)
	rightTotal, rightOnly := directionalDifference(right, left, limit)
	return model.StructuralEvidence{
		Intersection: intersection,
		Union:        union,
		LeftOnly: model.ProfileDifference{
			Fingerprint: leftFingerprint,
			Total:       leftTotal,
			Features:    leftOnly,
		},
		RightOnly: model.ProfileDifference{
			Fingerprint: rightFingerprint,
			Total:       rightTotal,
			Features:    rightOnly,
		},
	}
}

func directionalDifference(
	left model.FeatureBag,
	right model.FeatureBag,
	limit int,
) (int, []model.SharedFeature) {
	total := 0
	features := make([]model.SharedFeature, 0)
	for feature, leftCount := range left {
		count := leftCount - right[feature]
		if count <= 0 {
			continue
		}
		total += count
		features = append(features, model.SharedFeature{Feature: feature, Count: count})
	}
	sort.Slice(features, func(i, j int) bool {
		if features[i].Count == features[j].Count {
			return features[i].Feature < features[j].Feature
		}
		return features[i].Count > features[j].Count
	})
	if limit > 0 && len(features) > limit {
		features = features[:limit]
	}
	return total, features
}
