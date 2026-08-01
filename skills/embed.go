// Package skills embeds the official Agent Skills distributed with Mori.
package skills

import (
	"embed"
	"io/fs"
)

// ReviewSimilarityName is the directory and frontmatter name of Mori's
// structural-similarity review skill.
const ReviewSimilarityName = "mori-review-similarity"

//go:embed mori-review-similarity/SKILL.md mori-review-similarity/agents/openai.yaml
var embedded embed.FS

// ReviewSimilarity returns the complete portable Agent Skill package.
func ReviewSimilarity() (fs.FS, error) {
	return fs.Sub(embedded, ReviewSimilarityName)
}
