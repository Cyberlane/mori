// Package support records explicitly requested, source-free diagnostic sessions.
package support

const SchemaVersion = 1
const MaxSessionBytes = 64 * 1024
const MaxBundleBytes = 128 * 1024

type Session struct {
	ReportAvailable  bool              `json:"report_available"`
	SchemaVersion    int               `json:"schema_version"`
	Tool             Build             `json:"tool"`
	Status           string            `json:"status"`
	Failure          *Failure          `json:"failure"`
	Settings         *Settings         `json:"settings"`
	Counts           Counts            `json:"counts"`
	Phases           []Phase           `json:"phases"`
	Languages        []LanguageCount   `json:"languages"`
	ReviewedFindings []ReviewedFinding `json:"reviewed_findings,omitempty"`
}

type Build struct {
	Modified             bool   `json:"modified"`
	Version              string `json:"version"`
	Revision             string `json:"revision"`
	GOOS                 string `json:"goos"`
	GOARCH               string `json:"goarch"`
	GoVersion            string `json:"go_version"`
	ReportSchema         int    `json:"report_schema"`
	NormalizationVersion int    `json:"normalization_version"`
	ConfigSchema         int    `json:"config_schema"`
	ContractSchema       int    `json:"contract_schema"`
}
type Failure struct {
	Code  string `json:"code"`
	Stage string `json:"stage"`
}
type Phase struct {
	Name         string `json:"name"`
	Milliseconds int64  `json:"milliseconds"`
}
type LanguageCount struct {
	Language  string `json:"language"`
	Files     int64  `json:"files"`
	Fragments int64  `json:"fragments"`
}
type Counts struct {
	LeadingGroups          int64 `json:"leading_groups"`
	LeadingTestStoryGroups int64 `json:"leading_test_story_groups"`
	Files                  int64 `json:"files"`
	Fragments              int64 `json:"fragments"`
	CandidatePairs         int64 `json:"candidate_pairs"`
	LocationPairs          int64 `json:"location_pairs"`
	MatchGroups            int64 `json:"match_groups"`
	Warnings               int64 `json:"warnings"`
	ParseDiagnostics       int64 `json:"parse_diagnostics"`
	TestFiles              int64 `json:"test_files"`
	StoryFiles             int64 `json:"story_files"`
	TestFragments          int64 `json:"test_fragments"`
	StoryFragments         int64 `json:"story_fragments"`
	ZeroFragmentFiles      int64 `json:"zero_fragment_files"`
	GeneratedExcludedFiles int64 `json:"generated_excluded_files"`
	Truncated              bool  `json:"truncated"`
}
type Settings struct {
	Profile               string  `json:"profile"`
	Threshold             float64 `json:"threshold"`
	MinTokens             int64   `json:"min_tokens"`
	MaxGroups             int64   `json:"max_groups"`
	MaxOccurrences        int64   `json:"max_occurrences"`
	MaxPairs              int64   `json:"max_pairs"`
	MaxFileBytes          int64   `json:"max_file_bytes"`
	Workers               int64   `json:"workers"`
	ComparisonDomain      string  `json:"comparison_domain"`
	SQLDialect            string  `json:"sql_dialect"`
	FragmentSelection     string  `json:"fragment_selection"`
	Ranking               string  `json:"ranking"`
	SameLanguageOnly      bool    `json:"same_language_only"`
	CrossLanguageOnly     bool    `json:"cross_language_only"`
	EmbeddedSQL           bool    `json:"embedded_sql"`
	StatementBlocks       bool    `json:"statement_blocks"`
	BlockStatements       int64   `json:"block_statements"`
	MaxBlocksPerFunction  int64   `json:"max_blocks_per_function"`
	ExcludeGenerated      bool    `json:"exclude_generated"`
	RespectIgnore         bool    `json:"respect_ignore"`
	FailOnMatch           bool    `json:"fail_on_match"`
	RequireCoverage       bool    `json:"require_coverage"`
	MinFileCoverage       float64 `json:"min_file_coverage"`
	MaxZeroFragmentFiles  int64   `json:"max_zero_fragment_files"`
	FailOnWarning         bool    `json:"fail_on_warning"`
	FailOnParseDiagnostic bool    `json:"fail_on_parse_diagnostic"`
	ScopeSelected         bool    `json:"scope_selected"`
	RootCount             int64   `json:"root_count"`
	ExcludePatternCount   int64   `json:"exclude_pattern_count"`
	ProductionPathCount   int64   `json:"production_path_count,omitempty"`
	TestPathCount         int64   `json:"test_path_count,omitempty"`
	PriorityPathCount     int64   `json:"priority_path_count"`
	LanguagePairCount     int64   `json:"language_pair_count"`
	BaselineEnabled       bool    `json:"baseline_enabled"`
}

type ReviewedFinding struct {
	Rank           int    `json:"rank"`
	Classification string `json:"classification"`
}
