// Package config loads Mori's strict project scan configuration.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileName is the conventional project configuration file.
const FileName = ".mori.json"

const maxBytes = 1024 * 1024

// Settings contains optional scan values. Pointer fields distinguish omitted
// values from explicit zero or false values.
type Settings struct {
	Profile           string   `json:"profile,omitempty"`
	Threshold         *float64 `json:"threshold,omitempty"`
	MinTokens         *int     `json:"min_tokens,omitempty"`
	MaxGroups         *int     `json:"max_groups,omitempty"`
	MaxOccurrences    *int     `json:"max_occurrences,omitempty"`
	MaxPairs          *int     `json:"max_pairs,omitempty"`
	MaxFileBytes      *int64   `json:"max_file_bytes,omitempty"`
	Workers           *int     `json:"workers,omitempty"`
	Format            *string  `json:"format,omitempty"`
	ComparisonDomain  string   `json:"comparison_domain,omitempty"`
	SQLDialect        string   `json:"sql_dialect,omitempty"`
	Ranking           string   `json:"ranking,omitempty"`
	SameLanguageOnly  *bool    `json:"same_language_only,omitempty"`
	CrossLanguageOnly *bool    `json:"cross_language_only,omitempty"`
	LanguagePairs     []string `json:"language_pairs,omitempty"`
	FailOnMatch       *bool    `json:"fail_on_match,omitempty"`
	RequireCoverage   *bool    `json:"require_coverage,omitempty"`
	ExcludeGenerated  *bool    `json:"exclude_generated,omitempty"`
	Baseline          string   `json:"baseline,omitempty"`
	BaselineScope     string   `json:"baseline_scope,omitempty"`
	Excludes          []string `json:"exclude,omitempty"`
	RespectIgnore     *bool    `json:"respect_ignore,omitempty"`
}

// Discover searches start and its parents for FileName.
func Discover(start string) (string, bool, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(current)
	if err == nil && !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		candidate := filepath.Join(current, FileName)
		info, err := os.Lstat(candidate)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return "", false, fmt.Errorf("%s is not a regular file", FileName)
			}
			return candidate, true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", false, fmt.Errorf("inspect %s: %w", FileName, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false, nil
		}
		current = parent
	}
}

// Load decodes one strict configuration file.
func Load(path string) (Settings, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Settings{}, fmt.Errorf("read config: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Settings{}, errors.New("config is not a regular file")
	}
	if info.Size() > maxBytes {
		return Settings{}, fmt.Errorf("config exceeds %d bytes", maxBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return Settings{}, fmt.Errorf("read config: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return Settings{}, fmt.Errorf("read config: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return Settings{}, errors.New("config changed identity while opening")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return Settings{}, fmt.Errorf("read config: %w", err)
	}
	if len(content) > maxBytes {
		return Settings{}, fmt.Errorf("config exceeds %d bytes while reading", maxBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var settings Settings
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, fmt.Errorf("decode config: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return errors.New("decode config: multiple JSON values")
}
