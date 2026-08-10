// Package baseline stores reviewed-and-accepted Mori match candidates.
package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
)

// SchemaVersion is the current baseline file format.
const SchemaVersion = 3

// Scope controls whether acceptance follows normalized content everywhere or
// only the reviewed pair of source paths.
type Scope string

const (
	ScopeContent Scope = "content"
	ScopePath    Scope = "path"
)

var classifications = map[string]struct{}{
	"":                      {},
	"intentional":           {},
	"necessary-duplication": {},
	"test-fixture":          {},
	"generated":             {},
	"other":                 {},
}

// Entry records a candidate accepted for a review workflow.
type Entry struct {
	ID             string         `json:"id"`
	Similarity     float64        `json:"similarity"`
	Left           model.Location `json:"left"`
	Right          model.Location `json:"right"`
	Classification string         `json:"classification,omitempty"`
	Note           string         `json:"note,omitempty"`
}

// ScanProfile records every stable scan option that can change the candidate
// universe or make comparison coverage incomplete. Presentation-only options
// are intentionally excluded.
type ScanProfile struct {
	Scope             string       `json:"scope,omitempty"`
	ScopeRoots        []string     `json:"scope_roots,omitempty"`
	Threshold         float64      `json:"threshold"`
	MinTokens         int          `json:"min_tokens"`
	MaxPairs          int          `json:"max_pairs"`
	MaxFileBytes      int64        `json:"max_file_bytes"`
	ComparisonDomain  string       `json:"comparison_domain"`
	SQLDialect        string       `json:"sql_dialect"`
	EmbeddedSQL       bool         `json:"embedded_sql"`
	StatementBlocks   bool         `json:"statement_blocks"`
	BlockStatements   int          `json:"block_statements"`
	MaxBlocksPerFunc  int          `json:"max_blocks_per_function"`
	SameLanguageOnly  bool         `json:"same_language_only"`
	CrossLanguageOnly bool         `json:"cross_language_only"`
	LanguagePairs     []string     `json:"language_pairs"`
	ExcludeGenerated  bool         `json:"exclude_generated"`
	Excludes          []string     `json:"excludes"`
	RespectIgnore     bool         `json:"respect_ignore"`
	IgnoreFiles       []IgnoreFile `json:"ignore_files"`
	RequireCoverage   bool         `json:"require_coverage"`
	MinFileCoverage   float64      `json:"min_file_coverage"`
	MaxZeroFiles      int          `json:"max_zero_fragment_files"`
	FailOnWarning     bool         `json:"fail_on_warning"`
	FailOnDiagnostic  bool         `json:"fail_on_parse_diagnostic"`
}

// IgnoreFile binds an effective ignore-file path to the exact content Mori
// used during source discovery.
type IgnoreFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// Set is an in-memory baseline lookup.
type Set struct {
	schemaVersion int
	scope         Scope
	threshold     float64
	profile       ScanProfile
	profileDigest string
	entries       map[string]Entry
}

type document struct {
	SchemaVersion        int          `json:"schema_version"`
	MoriVersion          string       `json:"mori_version"`
	NormalizationVersion int          `json:"normalization_version"`
	IdentityScope        Scope        `json:"identity_scope,omitempty"`
	Threshold            float64      `json:"threshold"`
	ScanProfileDigest    string       `json:"scan_profile_digest,omitempty"`
	ScanProfile          *ScanProfile `json:"scan_profile,omitempty"`
	Entries              []Entry      `json:"entries"`
}

// Load reads a baseline and fails closed when its format or normalization
// contract is not compatible with the current binary. Schema 1 is interpreted
// using its original content-addressed behavior.
func Load(path string) (Set, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Set{}, fmt.Errorf("read baseline: %w", err)
	}

	var stored document
	if err := json.Unmarshal(content, &stored); err != nil {
		return Set{}, fmt.Errorf("decode baseline: %w", err)
	}
	if stored.SchemaVersion != 1 && stored.SchemaVersion != 2 && stored.SchemaVersion != SchemaVersion {
		return Set{}, fmt.Errorf(
			"baseline schema version %d is unsupported; expected 1, 2, or %d",
			stored.SchemaVersion,
			SchemaVersion,
		)
	}
	if stored.NormalizationVersion != normalize.Version {
		return Set{}, fmt.Errorf(
			"baseline normalization version %d does not match current version %d; migrate or regenerate the baseline",
			stored.NormalizationVersion,
			normalize.Version,
		)
	}

	scope := stored.IdentityScope
	if stored.SchemaVersion == 1 || scope == "" {
		scope = ScopeContent
	}
	if err := ValidateScope(scope); err != nil {
		return Set{}, err
	}
	entries := make(map[string]Entry, len(stored.Entries))
	for index, entry := range stored.Entries {
		if entry.ID == "" {
			return Set{}, fmt.Errorf("baseline entry %d has no ID", index+1)
		}
		if stored.SchemaVersion == SchemaVersion {
			if err := ValidateClassification(entry.Classification); err != nil {
				return Set{}, fmt.Errorf("baseline entry %d: %w", index+1, err)
			}
		}
		key := entryKey(scope, entry.ID, entry.Left, entry.Right)
		if _, exists := entries[key]; exists {
			return Set{}, fmt.Errorf("baseline contains duplicate identity %q", key)
		}
		entries[key] = entry
	}
	set := Set{
		schemaVersion: stored.SchemaVersion,
		scope:         scope,
		threshold:     stored.Threshold,
		entries:       entries,
	}
	if stored.SchemaVersion == SchemaVersion {
		if stored.ScanProfile == nil || stored.ScanProfileDigest == "" {
			return Set{}, errors.New("baseline schema 3 requires scan-profile evidence")
		}
		profile := normalizedProfile(*stored.ScanProfile)
		if err := validateProfile(profile); err != nil {
			return Set{}, fmt.Errorf("invalid baseline scan profile: %w", err)
		}
		digest := Digest(profile)
		if digest != stored.ScanProfileDigest {
			return Set{}, errors.New("baseline scan-profile digest does not match its stored profile")
		}
		if profile.Threshold != stored.Threshold {
			return Set{}, errors.New("baseline threshold does not match its stored scan profile")
		}
		set.profile = profile
		set.profileDigest = digest
	}
	return set, nil
}

// Digest returns the stable SHA-256 identity of one effective scan profile.
func Digest(profile ScanProfile) string {
	profile = normalizedProfile(profile)
	content, err := json.Marshal(profile)
	if err != nil {
		panic(fmt.Sprintf("encode scan profile: %v", err))
	}
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func normalizedProfile(profile ScanProfile) ScanProfile {
	profile.LanguagePairs = uniqueSorted(profile.LanguagePairs)
	profile.Excludes = uniqueSorted(profile.Excludes)
	profile.IgnoreFiles = append([]IgnoreFile{}, profile.IgnoreFiles...)
	sort.Slice(profile.IgnoreFiles, func(left, right int) bool {
		if profile.IgnoreFiles[left].Path != profile.IgnoreFiles[right].Path {
			return profile.IgnoreFiles[left].Path < profile.IgnoreFiles[right].Path
		}
		return profile.IgnoreFiles[left].Digest < profile.IgnoreFiles[right].Digest
	})
	if len(profile.IgnoreFiles) > 1 {
		unique := profile.IgnoreFiles[:1]
		for _, evidence := range profile.IgnoreFiles[1:] {
			previous := unique[len(unique)-1]
			if evidence != previous {
				unique = append(unique, evidence)
			}
		}
		profile.IgnoreFiles = unique
	}
	return profile
}

func uniqueSorted(values []string) []string {
	values = append([]string{}, values...)
	sort.Strings(values)
	if len(values) < 2 {
		return values
	}
	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

// ValidateScope checks a baseline identity scope.
func ValidateScope(scope Scope) error {
	if scope != ScopeContent && scope != ScopePath {
		return fmt.Errorf("baseline identity scope %q is unsupported; expected content or path", scope)
	}
	return nil
}

// ValidateClassification checks a durable review classification.
func ValidateClassification(value string) error {
	if _, ok := classifications[value]; !ok {
		return fmt.Errorf(
			"baseline classification %q is unsupported; expected intentional, necessary-duplication, test-fixture, generated, other, or empty",
			value,
		)
	}
	return nil
}

// New returns an empty schema-3 baseline using one reviewed scan profile.
func New(scope Scope, profile ScanProfile) (Set, error) {
	if err := ValidateScope(scope); err != nil {
		return Set{}, err
	}
	profile = normalizedProfile(profile)
	if err := validateProfile(profile); err != nil {
		return Set{}, err
	}
	return Set{
		schemaVersion: SchemaVersion,
		scope:         scope,
		threshold:     profile.Threshold,
		profile:       profile,
		profileDigest: Digest(profile),
		entries:       make(map[string]Entry),
	}, nil
}

// Scope returns the accepted identity scope.
func (set Set) Scope() Scope {
	return set.scope
}

// Legacy reports whether this baseline predates scan-profile evidence.
func (set Set) Legacy() bool {
	return set.schemaVersion < SchemaVersion
}

// ProfileDigest returns the stored scan-profile digest, or an empty string for
// legacy schema files.
func (set Set) ProfileDigest() string {
	return set.profileDigest
}

// Compatible reports whether the active scan profile matches this baseline.
func (set Set) Compatible(profile ScanProfile) bool {
	return !set.Legacy() && set.profileDigest == Digest(profile)
}

// Entries returns a deterministic copy of every accepted entry.
func (set Set) Entries() []Entry {
	entries := make([]Entry, 0, len(set.entries))
	for _, entry := range set.entries {
		entries = append(entries, entry)
	}
	sortEntries(entries, set.scope)
	return entries
}

// Match reports whether a source pair is accepted by this baseline.
func (set Set) Match(id string, left model.Location, right model.Location) bool {
	_, ok := set.entries[entryKey(set.scope, id, left, right)]
	return ok
}

// Has reports whether any baseline entry uses the content-pair ID.
func (set Set) Has(id string) bool {
	for _, entry := range set.entries {
		if entry.ID == id {
			return true
		}
	}
	return false
}

// Write replaces path atomically with the accepted groups in report.
func Write(
	path string,
	report model.Report,
	scope Scope,
	profile ScanProfile,
	previous *Set,
) error {
	if report.Truncated {
		return errors.New("cannot write a baseline from a truncated report")
	}
	if err := ValidateScope(scope); err != nil {
		return err
	}
	entries := entriesFromReport(report, scope)
	if previous != nil {
		preserveEntryMetadata(entries, *previous, scope)
	}
	return writeEntries(path, entries, report.Threshold, scope, profile)
}

// Add accepts every exact entry represented by one content identity in the
// active complete report. Path-scoped baselines retain all currently scored
// path pairs for that identity.
func Add(
	path string,
	set Set,
	report model.Report,
	identity string,
	note *string,
	classification *string,
	profile ScanProfile,
) (int, int, error) {
	if report.Truncated {
		return 0, 0, errors.New("cannot add to a baseline from a truncated report")
	}
	if identity == "" {
		return 0, 0, errors.New("baseline identity cannot be empty")
	}
	if set.Legacy() {
		return 0, 0, errors.New("legacy baseline must be explicitly migrated before mutation")
	}
	if !set.Compatible(profile) {
		return 0, 0, errors.New("baseline scan profile differs from the active scan")
	}
	if classification != nil {
		if err := ValidateClassification(*classification); err != nil {
			return 0, 0, err
		}
	}
	candidates := entriesFromReport(report, set.scope)
	added := 0
	updated := 0
	matched := 0
	for _, candidate := range candidates {
		if candidate.ID != identity {
			continue
		}
		matched++
		key := entryKey(set.scope, candidate.ID, candidate.Left, candidate.Right)
		entry, exists := set.entries[key]
		if !exists {
			entry = candidate
			added++
		} else {
			entry.Similarity = candidate.Similarity
			entry.Left = candidate.Left
			entry.Right = candidate.Right
			updated++
		}
		if note != nil {
			entry.Note = *note
		}
		if classification != nil {
			entry.Classification = *classification
		}
		set.entries[key] = entry
	}
	if matched == 0 {
		return 0, 0, fmt.Errorf("identity %q was not found in the active scan", identity)
	}
	return added, updated, writeEntries(
		path, set.Entries(), report.Threshold, set.scope, profile,
	)
}

// Remove deletes every accepted entry using one content identity.
func Remove(path string, set Set, identity string) (int, error) {
	if set.Legacy() {
		return 0, errors.New("legacy baseline must be explicitly migrated before mutation")
	}
	removed := 0
	for key, entry := range set.entries {
		if entry.ID == identity {
			delete(set.entries, key)
			removed++
		}
	}
	if removed == 0 {
		return 0, fmt.Errorf("identity %q is not present in the baseline", identity)
	}
	return removed, writeEntries(
		path, set.Entries(), set.threshold, set.scope, set.profile,
	)
}

// Edit updates durable human review metadata without changing acceptance.
func Edit(
	path string,
	set Set,
	identity string,
	note *string,
	classification *string,
) (int, error) {
	if set.Legacy() {
		return 0, errors.New("legacy baseline must be explicitly migrated before mutation")
	}
	if note == nil && classification == nil {
		return 0, errors.New("baseline edit requires --note or --classification")
	}
	if classification != nil {
		if err := ValidateClassification(*classification); err != nil {
			return 0, err
		}
	}
	updated := 0
	for key, entry := range set.entries {
		if entry.ID != identity {
			continue
		}
		if note != nil {
			entry.Note = *note
		}
		if classification != nil {
			entry.Classification = *classification
		}
		set.entries[key] = entry
		updated++
	}
	if updated == 0 {
		return 0, fmt.Errorf("identity %q is not present in the baseline", identity)
	}
	return updated, writeEntries(
		path, set.Entries(), set.threshold, set.scope, set.profile,
	)
}

// Migrate attaches explicitly accepted scan-profile evidence while preserving
// every existing entry and its metadata.
func Migrate(path string, set Set, profile ScanProfile) error {
	return writeEntries(path, set.Entries(), profile.Threshold, set.scope, profile)
}

// Preview summarizes the exact effect of replacing a baseline with every
// candidate in one complete report.
type Preview struct {
	Candidates int
	New        int
	Retained   int
	Removed    int
}

// ReplacementPreview returns deterministic replacement counts.
func ReplacementPreview(set *Set, report model.Report, scope Scope) Preview {
	candidates := entriesFromReport(report, scope)
	preview := Preview{Candidates: len(candidates)}
	if set == nil {
		preview.New = len(candidates)
		return preview
	}
	current := make(map[string]struct{}, len(candidates))
	for _, entry := range candidates {
		key := entryKey(scope, entry.ID, entry.Left, entry.Right)
		current[key] = struct{}{}
		if _, exists := set.entries[key]; exists {
			preview.Retained++
		} else {
			preview.New++
		}
	}
	for key := range set.entries {
		if _, exists := current[key]; !exists {
			preview.Removed++
		}
	}
	return preview
}

// Prune removes baseline entries absent from the current untruncated report
// without adding newly discovered candidates.
func Prune(path string, set Set, report model.Report, profile ScanProfile) error {
	if report.Truncated {
		return errors.New("cannot prune a baseline from a truncated report")
	}
	current := entrySet(report, set.scope)
	entries := make([]Entry, 0, len(set.entries))
	for key, entry := range set.entries {
		if _, exists := current[key]; exists {
			entries = append(entries, entry)
		}
	}
	return writeEntries(path, entries, report.Threshold, set.scope, profile)
}

func writeEntries(
	path string,
	entries []Entry,
	threshold float64,
	scope Scope,
	profile ScanProfile,
) error {
	profile = normalizedProfile(profile)
	if err := validateProfile(profile); err != nil {
		return fmt.Errorf("invalid scan profile: %w", err)
	}
	if profile.Threshold != threshold {
		return errors.New("baseline threshold does not match the active scan profile")
	}
	for index, entry := range entries {
		if err := ValidateClassification(entry.Classification); err != nil {
			return fmt.Errorf("baseline entry %d: %w", index+1, err)
		}
		if len(entry.Note) > 4096 {
			return fmt.Errorf("baseline entry %d note exceeds 4096 bytes", index+1)
		}
	}
	sortEntries(entries, scope)
	digest := Digest(profile)
	stored := document{
		SchemaVersion:        SchemaVersion,
		MoriVersion:          buildinfo.Version,
		NormalizationVersion: normalize.Version,
		IdentityScope:        scope,
		Threshold:            threshold,
		ScanProfileDigest:    digest,
		ScanProfile:          &profile,
		Entries:              entries,
	}
	content, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("encode baseline: %w", err)
	}
	content = append(content, '\n')
	if err := writeAtomically(path, content); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

func validateProfile(profile ScanProfile) error {
	for index, evidence := range profile.IgnoreFiles {
		if evidence.Path == "" {
			return fmt.Errorf("ignore file %d has no path", index+1)
		}
		decoded, err := hex.DecodeString(evidence.Digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("ignore file %q has an invalid SHA-256 digest", evidence.Path)
		}
	}
	return nil
}

func sortEntries(entries []Entry, scope Scope) {
	sort.Slice(entries, func(i, j int) bool {
		return entryKey(scope, entries[i].ID, entries[i].Left, entries[i].Right) <
			entryKey(scope, entries[j].ID, entries[j].Left, entries[j].Right)
	})
}

func preserveEntryMetadata(entries []Entry, previous Set, scope Scope) {
	for index := range entries {
		key := entryKey(scope, entries[index].ID, entries[index].Left, entries[index].Right)
		if prior, exists := previous.entries[key]; exists {
			entries[index].Classification = prior.Classification
			entries[index].Note = prior.Note
		}
	}
}

// Stale returns baseline entries absent from the current untruncated report.
func Stale(set Set, report model.Report) []Entry {
	current := entrySet(report, set.scope)
	stale := make([]Entry, 0)
	for key, entry := range set.entries {
		if _, exists := current[key]; !exists {
			stale = append(stale, entry)
		}
	}
	sort.Slice(stale, func(i, j int) bool {
		return entryKey(set.scope, stale[i].ID, stale[i].Left, stale[i].Right) <
			entryKey(set.scope, stale[j].ID, stale[j].Left, stale[j].Right)
	})
	return stale
}

func entrySet(report model.Report, scope Scope) map[string]struct{} {
	entries := entriesFromReport(report, scope)
	result := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		result[entryKey(scope, entry.ID, entry.Left, entry.Right)] = struct{}{}
	}
	return result
}

func entriesFromReport(report model.Report, scope Scope) []Entry {
	byKey := make(map[string]Entry)
	for _, group := range report.Groups {
		if scope == ScopeContent {
			left, right, ok := representativePair(group)
			if !ok {
				continue
			}
			entry := Entry{ID: group.ID, Similarity: group.Similarity, Left: left, Right: right}
			byKey[entryKey(scope, entry.ID, left, right)] = entry
			continue
		}
		for _, pair := range pathPairs(group) {
			entry := Entry{
				ID:         group.ID,
				Similarity: group.Similarity,
				Left:       pair[0],
				Right:      pair[1],
			}
			byKey[entryKey(scope, entry.ID, entry.Left, entry.Right)] = entry
		}
	}
	entries := make([]Entry, 0, len(byKey))
	for _, entry := range byKey {
		entries = append(entries, entry)
	}
	return entries
}

func representativePair(group model.MatchGroup) (model.Location, model.Location, bool) {
	if len(group.PathPairs) > 0 {
		return group.PathPairs[0].Left, group.PathPairs[0].Right, true
	}
	if len(group.Profiles) == 0 {
		return model.Location{}, model.Location{}, false
	}
	if len(group.Profiles) == 1 {
		if len(group.Profiles[0].Occurrences) < 2 {
			return model.Location{}, model.Location{}, false
		}
		return group.Profiles[0].Occurrences[0].Location,
			group.Profiles[0].Occurrences[1].Location, true
	}
	if len(group.Profiles[0].Occurrences) == 0 || len(group.Profiles[1].Occurrences) == 0 {
		return model.Location{}, model.Location{}, false
	}
	return group.Profiles[0].Occurrences[0].Location,
		group.Profiles[1].Occurrences[0].Location, true
}

func pathPairs(group model.MatchGroup) [][2]model.Location {
	result := make([][2]model.Location, 0)
	if len(group.PathPairs) > 0 {
		for _, pair := range group.PathPairs {
			result = append(result, [2]model.Location{pair.Left, pair.Right})
		}
		return result
	}
	if len(group.Profiles) == 0 {
		return result
	}
	if len(group.Profiles) == 1 {
		occurrences := group.Profiles[0].Occurrences
		for left := 0; left < len(occurrences); left++ {
			for right := left + 1; right < len(occurrences); right++ {
				result = append(result, [2]model.Location{
					occurrences[left].Location,
					occurrences[right].Location,
				})
			}
		}
		return result
	}
	for _, left := range group.Profiles[0].Occurrences {
		for _, right := range group.Profiles[1].Occurrences {
			result = append(result, [2]model.Location{left.Location, right.Location})
		}
	}
	return result
}

func entryKey(scope Scope, id string, left model.Location, right model.Location) string {
	if scope != ScopePath {
		return id
	}
	leftPath, rightPath := left.Path, right.Path
	if rightPath < leftPath {
		leftPath, rightPath = rightPath, leftPath
	}
	return id + "\x00" + leftPath + "\x00" + rightPath
}

func writeAtomically(path string, content []byte) (returnErr error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".mori-baseline-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()

	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}
