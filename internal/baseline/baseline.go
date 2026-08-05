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

// SchemaVersion is the version of the baseline file format.
const SchemaVersion = 1

// Entry records a candidate accepted for a review workflow. Locations are
// retained for human context and are never used to identify the entry.
type Entry struct {
	ID         string         `json:"id"`
	Similarity float64        `json:"similarity"`
	Left       model.Location `json:"left"`
	Right      model.Location `json:"right"`
	Note       string         `json:"note,omitempty"`
}

// Set is an in-memory baseline lookup.
type Set struct {
	entries map[string]Entry
}

type document struct {
	SchemaVersion        int     `json:"schema_version"`
	MoriVersion          string  `json:"mori_version"`
	NormalizationVersion int     `json:"normalization_version"`
	Threshold            float64 `json:"threshold"`
	Entries              []Entry `json:"entries"`
}

// Load reads a baseline and fails closed when its format or normalization
// contract is not compatible with the current binary.
func Load(path string) (Set, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Set{}, fmt.Errorf("read baseline: %w", err)
	}

	var stored document
	if err := json.Unmarshal(content, &stored); err != nil {
		return Set{}, fmt.Errorf("decode baseline: %w", err)
	}
	if stored.SchemaVersion != SchemaVersion {
		return Set{}, fmt.Errorf(
			"baseline schema version %d is unsupported; expected %d",
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

	entries := make(map[string]Entry, len(stored.Entries))
	for index, entry := range stored.Entries {
		if entry.ID == "" {
			return Set{}, fmt.Errorf("baseline entry %d has no ID", index+1)
		}
		if _, exists := entries[entry.ID]; exists {
			return Set{}, fmt.Errorf("baseline contains duplicate ID %q", entry.ID)
		}
		entries[entry.ID] = entry
	}
	return Set{entries: entries}, nil
}

// Has reports whether an accepted candidate ID is present in the baseline.
func (set Set) Has(id string) bool {
	_, ok := set.entries[id]
	return ok
}

// Write replaces path atomically with the accepted matches in report. The
// caller should provide an untruncated report when creating or pruning a
// baseline.
func Write(path string, report model.Report) error {
	if report.Truncated {
		return errors.New("cannot write a baseline from a truncated report")
	}
	byID := make(map[string]Entry, len(report.Matches))
	for index, match := range report.Matches {
		if match.ID == "" {
			return fmt.Errorf("match %d has no ID", index+1)
		}
		if _, exists := byID[match.ID]; exists {
			continue
		}
		byID[match.ID] = Entry{
			ID:         match.ID,
			Similarity: match.Similarity,
			Left:       match.Left.Location,
			Right:      match.Right.Location,
		}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, byID[id])
	}

	stored := document{
		SchemaVersion:        SchemaVersion,
		MoriVersion:          buildinfo.Version,
		NormalizationVersion: normalize.Version,
		Threshold:            report.Threshold,
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

// Stale returns baseline entries absent from the current untruncated report,
// ordered by their stable IDs.
func Stale(set Set, report model.Report) []Entry {
	current := make(map[string]struct{}, len(report.Matches))
	for _, match := range report.Matches {
		current[match.ID] = struct{}{}
	}

	stale := make([]Entry, 0)
	for id, entry := range set.entries {
		if _, exists := current[id]; !exists {
			stale = append(stale, entry)
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].ID < stale[j].ID })
	return stale
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
