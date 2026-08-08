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
