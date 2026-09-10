// Package feedback stores explicitly opted-in local measurements. It never sends data.
package feedback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Cyberlane/mori/internal/model"
)

const SchemaVersion = 1
const MaxRecords = 256
const maxBytes = 1024 * 1024
const staleStorageAge = 10 * time.Minute
const maxCleanupEntries = 512

// Sample contains only enumerations, booleans and coarse measurements. Never add
// report objects, free text, paths or content identities to this sharing schema.
type Sample struct {
	Policy            string            `json:"policy"`
	Analysis          string            `json:"analysis"`
	FindingsScope     string            `json:"findings_scope"`
	WarningCategories map[string]string `json:"warning_categories,omitempty"`
	Languages         map[string]string `json:"languages,omitempty"`
	ToolVersion       string            `json:"tool_version"`
	Cache             string            `json:"cache"`
	Classification    string            `json:"classification,omitempty"`
	Effort            string            `json:"effort,omitempty"`
	ReviewedRank      string            `json:"reviewed_rank,omitempty"`
	Duration          string            `json:"duration"`
	Files             string            `json:"files"`
	Fragments         string            `json:"fragments"`
	CandidatePairs    string            `json:"candidate_pairs"`
	Findings          string            `json:"findings"`
	Diagnostics       string            `json:"diagnostics"`
	Outcome           string            `json:"outcome"`
	Truncated         bool              `json:"truncated"`
}

type Bundle struct {
	SchemaVersion int      `json:"schema_version"`
	Samples       []Sample `json:"samples"`
}

type State struct {
	SchemaVersion int      `json:"schema_version"`
	Enabled       bool     `json:"enabled"`
	CI            bool     `json:"ci_enabled"`
	Samples       []Sample `json:"samples"`
}

type Store struct{ directory string }

// Open does not create storage. Project identity is local-only, derived from a
// canonical directory rather than repository metadata or a tracked config file.
func Open(base, project string) (*Store, error) {
	root, err := filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("feedback root must be a directory")
	}
	sum := sha256.Sum256([]byte(root))
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	return &Store{directory: filepath.Join(base, "mori", "feedback", hex.EncodeToString(sum[:]))}, nil
}

func Bucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n < 10:
		return "1-9"
	case n < 100:
		return "10-99"
	case n < 1000:
		return "100-999"
	default:
		return "1000+"
	}
}

var feedbackLanguages = map[string]bool{"bash": true, "c": true, "cpp": true, "csharp": true, "dart": true, "go": true, "java": true, "javascript": true, "typescript": true, "tsx": true, "kotlin": true, "lua": true, "luau": true, "php": true, "powershell": true, "python": true, "ruby": true, "rust": true, "sql": true, "swift": true, "zsh": true, "other": true}

var releaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)

func sanitizedVersion(version string) string {
	version = strings.TrimPrefix(version, "v")
	if releaseVersion.MatchString(version) {
		return version
	}
	return "dev"
}

func Measure(r model.Report, elapsed time.Duration, outcome string, cache ...string) Sample {
	duration := "under-1s"
	if elapsed >= 5*time.Minute {
		duration = "5m+"
	} else if elapsed >= time.Minute {
		duration = "1-5m"
	} else if elapsed >= 10*time.Second {
		duration = "10-59s"
	} else if elapsed >= time.Second {
		duration = "1-9s"
	}
	switch outcome {
	case "completed", "findings", "coverage", "error":
	default:
		outcome = "unknown"
	}
	cacheState := "bypassed"
	if len(cache) > 0 && (cache[0] == "hit" || cache[0] == "miss") {
		cacheState = cache[0]
	}
	languageCounts := make(map[string]int)
	for _, file := range r.FileCoverage {
		name := file.Language
		if !feedbackLanguages[name] {
			name = "other"
		}
		languageCounts[name]++
	}
	languages := make(map[string]string)
	for name, count := range languageCounts {
		languages[name] = Bucket(count)
	}
	policy, analysis := "scan", "unknown"
	if r.Review != nil {
		switch r.Review.Policy {
		case "strict", "advisory":
			policy = r.Review.Policy
		}
		switch r.Review.Analysis {
		case "complete", "incomplete", "no-comparable-source":
			analysis = r.Review.Analysis
		}
	}
	findings, findingsScope := r.TotalMatchGroups, "all"
	if r.Configuration.Focus != nil {
		findings = r.TotalFocusedMatchGroups
		findingsScope = "focused"
	}
	warningCounts := make(map[string]int)
	for _, warning := range r.Warnings {
		category := warning.Kind
		switch category {
		case "parse", "coverage", "baseline", "focus":
		default:
			category = "other"
		}
		warningCounts[category]++
	}
	warningCategories := make(map[string]string)
	for category, count := range warningCounts {
		warningCategories[category] = Bucket(count)
	}
	return Sample{Policy: policy, Analysis: analysis, FindingsScope: findingsScope, WarningCategories: warningCategories, Languages: languages, ToolVersion: sanitizedVersion(r.Tool.Version), Cache: cacheState, Duration: duration, Files: Bucket(r.Files), Fragments: Bucket(r.Fragments), CandidatePairs: Bucket(r.CandidatePairs), Findings: Bucket(findings), Diagnostics: Bucket(r.Coverage.ParseDiagnosticCount), Outcome: outcome, Truncated: r.Truncated}
}

func validSample(s Sample) bool {
	counts := map[string]bool{"0": true, "1-9": true, "10-99": true, "100-999": true, "1000+": true}
	durations := map[string]bool{"under-1s": true, "1-9s": true, "10-59s": true, "1-5m": true, "5m+": true}
	outcomes := map[string]bool{"completed": true, "findings": true, "coverage": true, "error": true, "unknown": true}
	classes := map[string]bool{"": true, "useful": true, "intentional": true, "false-positive": true, "uncertain": true}
	efforts := map[string]bool{"": true, "under-1m": true, "1-5m": true, "5-15m": true, "15m+": true}
	ranks := map[string]bool{"": true, "1-5": true, "6-10": true, "11-25": true, "26+": true}
	for name, count := range s.Languages {
		if !feedbackLanguages[name] || !counts[count] {
			return false
		}
	}
	policies := map[string]bool{"strict": true, "advisory": true, "scan": true}
	analyses := map[string]bool{"complete": true, "incomplete": true, "no-comparable-source": true, "unknown": true}
	warningKinds := map[string]bool{"parse": true, "coverage": true, "baseline": true, "focus": true, "other": true}
	for kind, count := range s.WarningCategories {
		if !warningKinds[kind] || !counts[count] {
			return false
		}
	}
	return policies[s.Policy] && analyses[s.Analysis] && (s.FindingsScope == "focused" || s.FindingsScope == "all") && (s.ToolVersion == "dev" || releaseVersion.MatchString(s.ToolVersion)) && (s.Cache == "hit" || s.Cache == "miss" || s.Cache == "bypassed") && classes[s.Classification] && (s.Classification == "" || s.Findings != "0") && efforts[s.Effort] && ranks[s.ReviewedRank] && (s.Classification != "" || (s.Effort == "" && s.ReviewedRank == "")) && durations[s.Duration] && outcomes[s.Outcome] && counts[s.Files] && counts[s.Fragments] && counts[s.CandidatePairs] && counts[s.Findings] && counts[s.Diagnostics]
}

// safeDirectory rejects symlinks along storage paths before any read or write.
// Storage is private to this OS user; concurrent operations fail closed via a lock.
func safeDirectory(path string, create bool) error {
	parent := filepath.Dir(path)
	if parent != path {
		if err := safeDirectory(parent, create); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && create {
		if err = os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe feedback storage directory")
	}
	return nil
}

func (s *Store) Read() (State, error) {
	empty := State{SchemaVersion: SchemaVersion, Samples: []Sample{}}
	if err := safeDirectory(s.directory, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return empty, nil
		}
		return empty, err
	}
	directoryInfo, err := os.Stat(s.directory)
	if err != nil {
		return empty, err
	}
	if runtime.GOOS != "windows" && directoryInfo.Mode().Perm()&0077 != 0 {
		return empty, errors.New("feedback directory must be private")
	}
	name := filepath.Join(s.directory, "state.json")
	info, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) || info.Size() > maxBytes {
		return empty, errors.New("unsafe feedback state file")
	}
	file, err := os.Open(name)
	if err != nil {
		return empty, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return empty, errors.New("feedback state changed while opening")
	}
	var state State
	decoder := json.NewDecoder(io.LimitReader(file, maxBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return empty, errors.New("invalid feedback state")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return empty, errors.New("invalid trailing feedback state")
	}
	if state.SchemaVersion != SchemaVersion || len(state.Samples) > MaxRecords {
		return empty, errors.New("unsupported feedback state")
	}
	for _, sample := range state.Samples {
		if !validSample(sample) {
			return empty, errors.New("invalid feedback sample")
		}
	}
	if state.Samples == nil {
		state.Samples = []Sample{}
	}
	return state, nil
}

func (s *Store) update(create bool, change func(*State) error) error {
	if err := safeDirectory(s.directory, create); err != nil {
		return err
	}
	info, err := os.Stat(s.directory)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("feedback directory must be private")
	}
	lock := filepath.Join(s.directory, "lock")
	if err := acquireStorageLock(lock); err != nil {
		return err
	}
	ownedLock, err := os.Lstat(lock)
	if err != nil {
		return errors.New("feedback storage lock unavailable")
	}
	defer func() {
		current, err := os.Lstat(lock)
		if err == nil && os.SameFile(ownedLock, current) {
			_ = os.Remove(lock)
		}
	}()
	cleanupStaleTemps(s.directory)
	state, err := s.Read()
	if err != nil {
		return err
	}
	if err := change(&state); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.directory, ".state-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(s.directory, "state.json"))
}

func (s *Store) Enable(ci bool) error {
	return s.update(true, func(state *State) error { state.Enabled = true; state.CI = ci; return nil })
}
func (s *Store) Disable() error {
	state, err := s.Read()
	if err != nil || !state.Enabled {
		return err
	}
	return s.update(false, func(state *State) error { state.Enabled = false; state.CI = false; return nil })
}

// Clear removes measurements while retaining the current consent decision.
func (s *Store) Clear() error {
	state, err := s.Read()
	if err != nil || len(state.Samples) == 0 {
		return err
	}
	return s.update(false, func(state *State) error { state.Samples = []Sample{}; return nil })
}
func (s *Store) Capture(sample Sample, ci bool) error {
	state, err := s.Read()
	if err != nil || !state.Enabled || (ci && !state.CI) {
		return err
	}
	if !validSample(sample) {
		return errors.New("invalid feedback sample")
	}
	return s.update(false, func(state *State) error {
		if !state.Enabled || (ci && !state.CI) {
			return nil
		}
		state.Samples = append(state.Samples, sample)
		if len(state.Samples) > MaxRecords {
			state.Samples = state.Samples[len(state.Samples)-MaxRecords:]
		}
		return nil
	})
}

// Export is also the preview. The caller controls where stdout goes; there is no
// upload mechanism and no automatic export during scans.
func (s *Store) Export(w io.Writer) error {
	state, err := s.Read()
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(Bundle{SchemaVersion: SchemaVersion, Samples: state.Samples}); err != nil {
		return fmt.Errorf("write feedback bundle: %w", err)
	}
	return nil
}

// ClassifyLatest records one explicitly reviewed finding from the latest scan.
// Reclassification replaces that annotation; it never accepts a baseline identity.
func (s *Store) ClassifyLatest(classification, effort string, rank int, ci bool) error {
	if classification == "" || rank < 0 {
		return errors.New("a valid classification and nonnegative rank are required")
	}
	rankBucket := ""
	switch {
	case rank == 0:
	case rank <= 5:
		rankBucket = "1-5"
	case rank <= 10:
		rankBucket = "6-10"
	case rank <= 25:
		rankBucket = "11-25"
	default:
		rankBucket = "26+"
	}
	probe := Measure(model.Report{TotalMatchGroups: 1}, 0, "completed")
	probe.Classification = classification
	probe.Effort = effort
	probe.ReviewedRank = rankBucket
	if !validSample(probe) {
		return errors.New("invalid feedback classification or effort")
	}
	return s.update(false, func(state *State) error {
		if !state.Enabled || (ci && !state.CI) {
			return errors.New("local feedback consent required")
		}
		if len(state.Samples) == 0 {
			return errors.New("no retained scan to classify")
		}
		latest := &state.Samples[len(state.Samples)-1]
		if latest.Findings == "0" {
			return errors.New("latest scan has no findings to classify")
		}
		latest.Classification = classification
		latest.Effort = effort
		latest.ReviewedRank = rankBucket
		return nil
	})
}

// acquireStorageLock makes at most one recovery attempt. Only an unchanged empty
// directory older than ten minutes can be removed; fresh writers are untouched.
func acquireStorageLock(path string) error {
	for attempt := 0; attempt < 2; attempt++ {
		if err := os.Mkdir(path, 0700); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrExist) {
			return errors.New("feedback storage busy or unavailable")
		}
		if attempt != 0 {
			break
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || time.Since(info.ModTime()) <= staleStorageAge {
			break
		}
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(info, current) || !current.ModTime().Equal(info.ModTime()) {
			break
		}
		// Remove refuses nonempty directories, including unexpected lock contents.
		if err := os.Remove(path); err != nil {
			break
		}
	}
	return errors.New("feedback storage busy or unavailable")
}

// Cleanup is deliberately bounded and best effort. It runs only with the lock;
// no filename can nominate a file outside this private storage directory.
func cleanupStaleTemps(directory string) {
	dir, err := os.Open(directory)
	if err != nil {
		return
	}
	defer dir.Close()
	entries, err := dir.ReadDir(maxCleanupEntries)
	if err != nil && err != io.EOF {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".state-") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) <= staleStorageAge {
			continue
		}
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(info, current) || !current.ModTime().Equal(info.ModTime()) {
			continue
		}
		_ = os.Remove(path)
	}
}
