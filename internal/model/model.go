// Package model defines the analyzer's stable internal and output models.
package model

// SchemaVersion is the current machine-readable report contract.
const SchemaVersion = 2

// FeatureBag is a multiset of normalized AST features.
type FeatureBag map[string]int

// Location identifies a function-like source fragment.
type Location struct {
	Path      string `json:"path"`
	Language  string `json:"language"`
	Name      string `json:"name"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Fragment is a normalized function-like syntax tree.
type Fragment struct {
	Location     Location   `json:"location"`
	TokenCount   int        `json:"token_count"`
	FeatureCount int        `json:"feature_count"`
	Fingerprint  string     `json:"fingerprint"`
	Features     FeatureBag `json:"-"`
}

// FragmentSummary is the public portion of a fragment in a report.
type FragmentSummary struct {
	Location     Location `json:"location"`
	TokenCount   int      `json:"token_count"`
	FeatureCount int      `json:"feature_count"`
	Fingerprint  string   `json:"fingerprint"`
}

// Summary returns the serializable portion of a fragment.
func (f Fragment) Summary() FragmentSummary {
	return FragmentSummary{
		Location:     f.Location,
		TokenCount:   f.TokenCount,
		FeatureCount: f.FeatureCount,
		Fingerprint:  f.Fingerprint,
	}
}

// SharedFeature explains one contributor to a similarity score.
type SharedFeature struct {
	Feature string `json:"feature"`
	Count   int    `json:"count"`
}

// Match is one pair at or above the configured similarity threshold.
type Match struct {
	ID             string          `json:"id"`
	Similarity     float64         `json:"similarity"`
	Left           FragmentSummary `json:"left"`
	Right          FragmentSummary `json:"right"`
	SharedFeatures []SharedFeature `json:"shared_features"`
}

// Warning records a recoverable discovery or parsing problem.
type Warning struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Report is the stable JSON and text reporting model.
type Report struct {
	SchemaVersion  int       `json:"schema_version"`
	Threshold      float64   `json:"threshold"`
	Files          int       `json:"files"`
	Fragments      int       `json:"fragments"`
	CandidatePairs int       `json:"candidate_pairs"`
	TotalMatches   int       `json:"total_matches"`
	Suppressed     int       `json:"suppressed"`
	Truncated      bool      `json:"truncated"`
	Matches        []Match   `json:"matches"`
	Warnings       []Warning `json:"warnings"`
}
