// Package model defines the analyzer's stable internal and output models.
package model

import "github.com/Cyberlane/mori/internal/buildinfo"

// SchemaVersion is the current machine-readable report contract.
const SchemaVersion = 18

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
	Location       Location   `json:"location"`
	StartByte      uint       `json:"-"`
	EndByte        uint       `json:"-"`
	TokenCount     int        `json:"token_count"`
	FeatureCount   int        `json:"feature_count"`
	Fingerprint    string     `json:"fingerprint"`
	NestingDepth   int        `json:"nesting_depth"`
	Parent         *Location  `json:"parent,omitempty"`
	ParentID       string     `json:"parent_fingerprint,omitempty"`
	NestedCount    int        `json:"nested_function_count"`
	Features       FeatureBag `json:"-"`
	LiteralDigests []string   `json:"-"`
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

// SharedFeature records one canonical feature and its weighted multiset count.
type SharedFeature struct {
	Feature string `json:"feature"`
	Count   int    `json:"count"`
}

// ProfileDifference records bounded features present more often in one
// normalized profile than the other. Total counts remain complete even when
// the feature list is bounded.
type ProfileDifference struct {
	Fingerprint string          `json:"fingerprint"`
	Total       int             `json:"weighted_only_total"`
	Features    []SharedFeature `json:"features"`
}

// StructuralEvidence explains the exact weighted Jaccard totals and bounded
// directional differences for one representative normalized profile pair.
type StructuralEvidence struct {
	Intersection int               `json:"weighted_intersection"`
	Union        int               `json:"weighted_union"`
	LeftOnly     ProfileDifference `json:"left_only"`
	RightOnly    ProfileDifference `json:"right_only"`
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
	ID              string             `json:"content_pair_id"`
	Similarity      float64            `json:"similarity"`
	LocationPairs   int                `json:"location_pairs"`
	Focused         bool               `json:"focused"`
	FocusedCount    int                `json:"focused_occurrences"`
	Profiles        []FragmentProfile  `json:"profiles"`
	ShapeSummary    []string           `json:"shape_summary"`
	SharedFeatures  []SharedFeature    `json:"shared_features"`
	Evidence        StructuralEvidence `json:"structural_evidence"`
	ReviewPriority  int                `json:"review_priority"`
	ReviewSignals   []string           `json:"review_signals"`
	LiteralEvidence *LiteralEvidence   `json:"literal_evidence,omitempty"`
	PathPairs       []LocationPair     `json:"-"`
}

// LiteralEvidence summarizes source-free literal-position comparisons for a
// match group. Literal values and their digests are never serialized.
type LiteralEvidence struct {
	ComparedPairs             int `json:"compared_location_pairs"`
	PairsWithDifferences      int `json:"pairs_with_differences"`
	MaxDifferingPositions     int `json:"max_differing_positions"`
	LiteralCountMismatchPairs int `json:"literal_count_mismatch_pairs"`
}

// PriorityPathRule is one deterministic presentation-only review-priority
// rule. It never changes structural scores or content identities.
type PriorityPathRule struct {
	Pattern  string `json:"pattern"`
	Priority int    `json:"priority"`
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
	IndexDigest          string                `json:"index_digest,omitempty"`
	WorkingTreeIncluded  bool                  `json:"working_tree_included"`
	UntrackedIncluded    bool                  `json:"untracked_included"`
	ChangedPaths         []string              `json:"changed_paths"`
	DeletedPaths         []string              `json:"deleted_paths"`
	Worktrees            []WorktreeFocusConfig `json:"worktrees,omitempty"`
	DiscoveredFocusFiles int                   `json:"discovered_focus_files"`
	RequiredFocusFiles   int                   `json:"required_focus_files"`
	CoveredFocusFiles    int                   `json:"covered_focus_files"`
	PathEvidence         []FocusPathEvidence   `json:"path_evidence"`
}

// FocusPathEvidence explains whether one requested review path was analyzed.
type FocusPathEvidence struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
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

// FileCoverage records how much comparison evidence one supported source file
// contributed at the configured token floor.
type FileCoverage struct {
	Path             string `json:"path"`
	Language         string `json:"language"`
	LanguageFamily   string `json:"language_family"`
	ComparisonDomain string `json:"comparison_domain"`
	Status           string `json:"status"`
	Generated        bool   `json:"generated"`
	GeneratedMarker  string `json:"generated_marker,omitempty"`
	FragmentCount    int    `json:"fragment_count"`
	CandidateCount   int    `json:"candidate_fragment_count"`
	BelowTokenFloor  int    `json:"below_token_floor_count"`
	ZeroReason       string `json:"zero_fragment_reason,omitempty"`
	SkippedFragments int    `json:"skipped_fragments"`
	ParseDiagnostics int    `json:"parse_diagnostics"`
}

// UnsupportedExtension records an aggregate discovery count without exposing
// individual unsupported paths.
type UnsupportedExtension struct {
	Extension string `json:"extension"`
	FileCount int    `json:"file_count"`
}

// CoverageSummary records deterministic evidence used by strict coverage
// policies. Generated exclusions are supported files but are not analyzed.
type CoverageSummary struct {
	SupportedFiles        int                    `json:"supported_files"`
	AnalyzedFiles         int                    `json:"analyzed_files"`
	FragmentFiles         int                    `json:"fragment_files"`
	ZeroFragmentFiles     int                    `json:"zero_fragment_files"`
	GeneratedExcluded     int                    `json:"generated_excluded_files"`
	WarningFiles          int                    `json:"warning_files"`
	WarningCount          int                    `json:"warning_count"`
	ParseDiagnosticFiles  int                    `json:"parse_diagnostic_files"`
	ParseDiagnosticCount  int                    `json:"parse_diagnostic_count"`
	UnsupportedExtensions []UnsupportedExtension `json:"unsupported_extensions"`
}

// EffectiveConfig records the scan inputs needed to reproduce discovery and
// pair selection.
type EffectiveConfig struct {
	Profile           string               `json:"profile,omitempty"`
	Scope             string               `json:"scope,omitempty"`
	ScopeRoots        []string             `json:"scope_roots,omitempty"`
	ConfigPath        string               `json:"config_path,omitempty"`
	IgnoreFiles       []string             `json:"ignore_files"`
	IgnoreEvidence    []IgnoreFileEvidence `json:"ignore_file_evidence"`
	RespectIgnore     bool                 `json:"respect_ignore"`
	ExcludeGenerated  bool                 `json:"exclude_generated"`
	Excludes          []string             `json:"excludes"`
	MinTokens         int                  `json:"min_tokens"`
	MaxGroups         int                  `json:"max_groups"`
	MaxOccurrences    int                  `json:"max_occurrences"`
	MaxPairs          int                  `json:"max_pairs"`
	MaxFileBytes      int64                `json:"max_file_bytes"`
	ComparisonDomain  string               `json:"comparison_domain"`
	SQLDialect        string               `json:"sql_dialect"`
	EmbeddedSQL       bool                 `json:"embedded_sql"`
	StatementBlocks   bool                 `json:"statement_blocks"`
	BlockStatements   int                  `json:"block_statements"`
	MaxBlocksPerFunc  int                  `json:"max_blocks_per_function"`
	Ranking           string               `json:"ranking"`
	PriorityPaths     []PriorityPathRule   `json:"priority_paths"`
	SameLanguageOnly  bool                 `json:"same_language_only"`
	CrossLanguageOnly bool                 `json:"cross_language_only"`
	LanguagePairs     []string             `json:"language_pairs"`
	RequireCoverage   bool                 `json:"require_coverage"`
	MinFileCoverage   float64              `json:"min_file_coverage"`
	MaxZeroFiles      int                  `json:"max_zero_fragment_files"`
	FailOnWarning     bool                 `json:"fail_on_warning"`
	FailOnDiagnostic  bool                 `json:"fail_on_parse_diagnostic"`
	BaselinePath      string               `json:"baseline_path,omitempty"`
	ScanProfileDigest string               `json:"scan_profile_digest"`
	BaselineDigest    string               `json:"baseline_profile_digest,omitempty"`
	BaselineStatus    string               `json:"baseline_profile_status,omitempty"`
	StdinPath         string               `json:"stdin_path,omitempty"`
	Focus             *FocusConfig         `json:"focus,omitempty"`
	Input             *InputSnapshot       `json:"input,omitempty"`
}

// InputSnapshot identifies the immutable input view used for a scan.
type InputSnapshot struct {
	Mode                string `json:"mode"`
	GitRoot             string `json:"git_root,omitempty"`
	HeadCommit          string `json:"head_commit,omitempty"`
	IndexDigest         string `json:"index_digest,omitempty"`
	WorkingTreeIncluded bool   `json:"working_tree_included"`
	UntrackedIncluded   bool   `json:"untracked_included"`
}

// IgnoreFileEvidence identifies the exact ignore content used during source
// discovery without embedding that content in a report.
type IgnoreFileEvidence struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
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
	FileCoverage            []FileCoverage  `json:"file_coverage"`
	Coverage                CoverageSummary `json:"coverage"`
	Configuration           EffectiveConfig `json:"configuration"`
}
