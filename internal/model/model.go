// Package model defines the analyzer's stable internal and output models.
package model

import "github.com/Cyberlane/mori/internal/buildinfo"

// SchemaVersion is the current machine-readable report contract.
const SchemaVersion = 8

// FeatureBag is a multiset of normalized AST features.
type FeatureBag map[string]int

// Location identifies one source fragment.
type Location struct {
	Path             string `json:"path"`
	Language         string `json:"language"`
	LanguageFamily   string `json:"language_family"`
	ComparisonDomain string `json:"comparison_domain"`
	FragmentKind     string `json:"fragment_kind"`
	Name             string `json:"name"`
	StartLine        int    `json:"start_line"`
	EndLine          int    `json:"end_line"`
}

// Fragment is one normalized source syntax tree.
type Fragment struct {
	Location     Location   `json:"location"`
	StartByte    uint       `json:"-"`
	EndByte      uint       `json:"-"`
	TokenCount   int        `json:"token_count"`
	FeatureCount int        `json:"feature_count"`
	Fingerprint  string     `json:"fingerprint"`
	NestingDepth int        `json:"nesting_depth"`
	Parent       *Location  `json:"parent,omitempty"`
	ParentID     string     `json:"parent_fingerprint,omitempty"`
	NestedCount  int        `json:"nested_function_count"`
	Features     FeatureBag `json:"-"`
}

// FragmentSummary is the public portion of a fragment in a report.
type FragmentSummary struct {
	Location     Location  `json:"location"`
	TokenCount   int       `json:"token_count"`
	FeatureCount int       `json:"feature_count"`
	Fingerprint  string    `json:"fingerprint"`
	NestingDepth int       `json:"nesting_depth"`
	Parent       *Location `json:"parent,omitempty"`
	ParentID     string    `json:"parent_fingerprint,omitempty"`
	NestedCount  int       `json:"nested_function_count"`
}

// Summary returns the serializable portion of a fragment.
func (f Fragment) Summary() FragmentSummary {
	return FragmentSummary{
		Location:     f.Location,
		TokenCount:   f.TokenCount,
		FeatureCount: f.FeatureCount,
		Fingerprint:  f.Fingerprint,
		NestingDepth: f.NestingDepth,
		Parent:       cloneLocation(f.Parent),
		ParentID:     f.ParentID,
		NestedCount:  f.NestedCount,
	}
}

func cloneLocation(location *Location) *Location {
	if location == nil {
		return nil
	}
	cloned := *location
	return &cloned
}

// SharedFeature explains one contributor to a similarity score.
type SharedFeature struct {
	Feature string `json:"feature"`
	Count   int    `json:"count"`
}

// FragmentProfile groups source occurrences with the same normalized content
// fingerprint.
type FragmentProfile struct {
	Fingerprint     string            `json:"fingerprint"`
	TokenCount      int               `json:"token_count"`
	FeatureCount    int               `json:"feature_count"`
	OccurrenceCount int               `json:"occurrence_count"`
	Occurrences     []FragmentSummary `json:"occurrences"`
}

// LocationPair records one exact qualifying source pair for internal baseline
// construction. Grouped JSON exposes occurrences and counts instead.
type LocationPair struct {
	Left  Location
	Right Location
}

// MatchGroup is one content-pair identity at or above the configured
// similarity threshold. One group can represent many source-location pairs.
type MatchGroup struct {
	ID             string            `json:"content_pair_id"`
	Similarity     float64           `json:"similarity"`
	LocationPairs  int               `json:"location_pairs"`
	Focused        bool              `json:"focused"`
	FocusedCount   int               `json:"focused_occurrences"`
	Profiles       []FragmentProfile `json:"profiles"`
	ShapeSummary   []string          `json:"shape_summary"`
	SharedFeatures []SharedFeature   `json:"shared_features"`
	PathPairs      []LocationPair    `json:"-"`
}

// WorktreeFocusConfig records one explicitly resolved Git worktree and keeps
// every path relative to that worktree root.
type WorktreeFocusConfig struct {
	Root                string   `json:"root"`
	RequestedBase       string   `json:"requested_base"`
	BaseCommit          string   `json:"base_commit"`
	MergeBase           string   `json:"merge_base"`
	HeadCommit          string   `json:"head_commit"`
	WorkingTreeIncluded bool     `json:"working_tree_included"`
	UntrackedIncluded   bool     `json:"untracked_included"`
	ChangedPaths        []string `json:"changed_paths"`
	DeletedPaths        []string `json:"deleted_paths"`
}

// FocusConfig records deterministic path focus inputs and their resolution.
type FocusConfig struct {
	Mode                 string                `json:"mode"`
	ExplicitPaths        []string              `json:"explicit_paths"`
	RequestedBase        string                `json:"requested_base,omitempty"`
	BaseCommit           string                `json:"base_commit,omitempty"`
	MergeBase            string                `json:"merge_base,omitempty"`
	HeadCommit           string                `json:"head_commit,omitempty"`
	WorkingTreeIncluded  bool                  `json:"working_tree_included"`
	UntrackedIncluded    bool                  `json:"untracked_included"`
	ChangedPaths         []string              `json:"changed_paths"`
	DeletedPaths         []string              `json:"deleted_paths"`
	Worktrees            []WorktreeFocusConfig `json:"worktrees,omitempty"`
	DiscoveredFocusFiles int                   `json:"discovered_focus_files"`
}

// ParseDiagnostic identifies one bounded Tree-sitter error or missing node.
type ParseDiagnostic struct {
	NodeKind    string `json:"node_kind"`
	StartLine   int    `json:"start_line"`
	StartColumn int    `json:"start_column"`
	EndLine     int    `json:"end_line"`
	EndColumn   int    `json:"end_column"`
}

// Warning records a recoverable discovery or parsing problem.
type Warning struct {
	Kind             string            `json:"kind,omitempty"`
	Path             string            `json:"path,omitempty"`
	Language         string            `json:"language,omitempty"`
	Message          string            `json:"message"`
	TotalDiagnostics int               `json:"total_diagnostics,omitempty"`
	SkippedFragments int               `json:"skipped_fragments,omitempty"`
	Diagnostics      []ParseDiagnostic `json:"diagnostics,omitempty"`
}

// EffectiveConfig records the scan inputs needed to reproduce discovery and
// pair selection.
type EffectiveConfig struct {
	ConfigPath        string       `json:"config_path,omitempty"`
	IgnoreFiles       []string     `json:"ignore_files"`
	RespectIgnore     bool         `json:"respect_ignore"`
	Excludes          []string     `json:"excludes"`
	MinTokens         int          `json:"min_tokens"`
	MaxGroups         int          `json:"max_groups"`
	MaxOccurrences    int          `json:"max_occurrences"`
	MaxPairs          int          `json:"max_pairs"`
	MaxFileBytes      int64        `json:"max_file_bytes"`
	ComparisonDomain  string       `json:"comparison_domain"`
	SQLDialect        string       `json:"sql_dialect"`
	SameLanguageOnly  bool         `json:"same_language_only"`
	CrossLanguageOnly bool         `json:"cross_language_only"`
	LanguagePairs     []string     `json:"language_pairs"`
	BaselinePath      string       `json:"baseline_path,omitempty"`
	Focus             *FocusConfig `json:"focus,omitempty"`
}

// Report is the stable JSON and text reporting model.
type Report struct {
	SchemaVersion           int             `json:"schema_version"`
	Tool                    buildinfo.Info  `json:"tool"`
	Threshold               float64         `json:"threshold"`
	Files                   int             `json:"files"`
	Fragments               int             `json:"fragments"`
	CandidatePairs          int             `json:"candidate_pairs"`
	TotalLocationPairs      int             `json:"total_location_pairs"`
	TotalMatchGroups        int             `json:"total_match_groups"`
	TotalFocusedMatchGroups int             `json:"total_focused_match_groups"`
	SuppressedLocationPairs int             `json:"suppressed_location_pairs"`
	SuppressedMatchGroups   int             `json:"suppressed_match_groups"`
	Truncated               bool            `json:"truncated"`
	Groups                  []MatchGroup    `json:"groups"`
	Warnings                []Warning       `json:"warnings"`
	Configuration           EffectiveConfig `json:"configuration"`
}
