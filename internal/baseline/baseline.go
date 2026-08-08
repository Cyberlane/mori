// Package baseline stores reviewed-and-accepted Mori match candidates.
package baseline

import (
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
const SchemaVersion = 2

// Scope controls whether acceptance follows normalized content everywhere or
// only the reviewed pair of source paths.
type Scope string

const (
	ScopeContent Scope = "content"
	ScopePath    Scope = "path"
)

// Entry records a candidate accepted for a review workflow.
type Entry struct {
	ID         string         `json:"id"`
	Similarity float64        `json:"similarity"`
	Left       model.Location `json:"left"`
	Right      model.Location `json:"right"`
	Note       string         `json:"note,omitempty"`
}

// Set is an in-memory baseline lookup.
type Set struct {
	scope   Scope
	entries map[string]Entry
}

type document struct {
	SchemaVersion        int     `json:"schema_version"`
	MoriVersion          string  `json:"mori_version"`
	NormalizationVersion int     `json:"normalization_version"`
	IdentityScope        Scope   `json:"identity_scope,omitempty"`
	Threshold            float64 `json:"threshold"`
	Entries              []Entry `json:"entries"`
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
	if stored.SchemaVersion != 1 && stored.SchemaVersion != SchemaVersion {
		return Set{}, fmt.Errorf(
			"baseline schema version %d is unsupported; expected 1 or %d",
			stored.SchemaVersion,
			SchemaVersion,
		)
	}
	if stored.NormalizationVersion != normalize.Version {
		return Set{}, fmt.Errorf(
			"baseline normalization version %d does not match current version %d; run baseline update",
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
		key := entryKey(scope, entry.ID, entry.Left, entry.Right)
		if _, exists := entries[key]; exists {
			return Set{}, fmt.Errorf("baseline contains duplicate identity %q", key)
		}
		entries[key] = entry
	}
	return Set{scope: scope, entries: entries}, nil
}

// ValidateScope checks a baseline identity scope.
func ValidateScope(scope Scope) error {
	if scope != ScopeContent && scope != ScopePath {
		return fmt.Errorf("baseline identity scope %q is unsupported; expected content or path", scope)
	}
	return nil
}

// Scope returns the accepted identity scope.
func (set Set) Scope() Scope {
	return set.scope
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
func Write(path string, report model.Report, scope Scope) error {
	if report.Truncated {
		return errors.New("cannot write a baseline from a truncated report")
	}
	if err := ValidateScope(scope); err != nil {
		return err
	}
	entries := entriesFromReport(report, scope)
	return writeEntries(path, entries, report.Threshold, scope)
}

// Prune removes baseline entries absent from the current untruncated report
// without adding newly discovered candidates.
func Prune(path string, set Set, report model.Report) error {
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
	return writeEntries(path, entries, report.Threshold, set.scope)
}

func writeEntries(path string, entries []Entry, threshold float64, scope Scope) error {
	sort.Slice(entries, func(i, j int) bool {
		return entryKey(scope, entries[i].ID, entries[i].Left, entries[i].Right) <
			entryKey(scope, entries[j].ID, entries[j].Left, entries[j].Right)
	})
	stored := document{
		SchemaVersion:        SchemaVersion,
		MoriVersion:          buildinfo.Version,
		NormalizationVersion: normalize.Version,
		IdentityScope:        scope,
		Threshold:            threshold,
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
