package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Cyberlane/mori/internal/analyzer"
	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/pathutil"
	"github.com/Cyberlane/mori/internal/projectcontract"
	"github.com/Cyberlane/mori/internal/support"
)

// Runtime-only capture state never enters a report, receipt, or feedback store.
// Only the explicit allowlist built in session is persisted.
type scanDiagnostics struct {
	ctx               context.Context
	path              string
	started           time.Time
	stage, reason     string
	requested         scanOptions
	effective         *scanOptions
	paths             []string
	result            *model.Report
	discovery         time.Duration
	discoveryObserved bool
	timings           analyzer.PhaseTimings
}

func (d *scanDiagnostics) finish(code int, stderr io.Writer) {
	if d.path == "" {
		return
	}
	if !d.safeDestination() {
		fmt.Fprintln(stderr, "mori: diagnostic session not written: destination conflicts with scan inputs or artifacts")
		return
	}
	if err := support.WriteSession(d.path, d.session(code)); err != nil {
		fmt.Fprintln(stderr, "mori: diagnostic session not written: destination unavailable or session invalid")
	}
}

func (d *scanDiagnostics) safeDestination() bool {
	target, err := diagnosticDestinationIdentity(d.path)
	if err != nil || filepath.Base(target) == config.FileName {
		return false
	}
	o := d.requested
	if d.effective != nil {
		o = *d.effective
	}
	protected := append([]string{}, d.paths...)
	protected = append(protected, o.requestedConfig, o.configPath, o.outputPath, o.baselinePath, o.reviewReceiptPath, o.stdinPath)
	for _, path := range protected {
		if path == "" {
			continue
		}
		absolute, err := diagnosticProtectedIdentity(path)
		if err != nil || absolute == target {
			return false
		}

	}
	return true
}

// Match the support writer's destination identity: resolve existing parent
// directories, including /tmp and user-created aliases, but never follow the
// final component. The writer independently refuses existing final components.
func diagnosticDestinationIdentity(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, suffix := filepath.Dir(absolute), filepath.Base(absolute)
	// Resolve the nearest existing ancestor for protected missing paths. The
	// support writer still independently requires its destination parent to exist.
	for range 128 {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(parent), suffix)
		parent = next
	}
	return "", fmt.Errorf("diagnostic path ancestry limit")
}

func diagnosticProtectedIdentity(path string) (string, error) {
	// Following a protected final link also protects its missing target: writing
	// that target must not turn a previously dangling input/config link into data.
	for range 32 {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return diagnosticDestinationIdentity(path)
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return diagnosticDestinationIdentity(path)
		}
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return "", fmt.Errorf("diagnostic protected link limit")
}

func (d *scanDiagnostics) session(code int) support.Session {
	info := buildinfo.Current()
	s := support.Session{SchemaVersion: support.SchemaVersion, Tool: support.SanitizeBuild(support.Build{Version: info.Version, Revision: info.Revision, Modified: info.Modified, GOOS: info.GOOS, GOARCH: info.GOARCH, GoVersion: info.GoVersion, ReportSchema: model.SchemaVersion, NormalizationVersion: normalize.Version, ConfigSchema: projectConfigSchemaVersion, ContractSchema: projectcontract.SchemaVersion}), Status: "error", Phases: []support.Phase{{Name: "total", Milliseconds: time.Since(d.started).Milliseconds()}}, Languages: []support.LanguageCount{}}
	switch code {
	case exitSuccess:
		s.Status = "success"
	case exitUsage:
		s.Status = "usage"
	case exitFindings:
		s.Status = "findings"
	case exitCoverage:
		s.Status = "coverage"
	case exitUpgrade:
		s.Status = "upgrade"
	}
	if code != exitSuccess && code != exitFindings {
		reason := "error"
		switch d.stage {
		case "arguments", "validation":
			reason = "invalid-arguments"
		case "configuration":
			reason = "configuration"
		case "coverage":
			reason = "coverage"
		case "project":
			if code == exitUpgrade {
				reason = "upgrade"
			} else {
				reason = "io"
			}
		case "output", "receipt", "baseline", "discovery":
			reason = "io"
		}
		if d.reason == "candidate_limit" {
			reason = "resource-limit"
		}
		if d.reason == "cancelled" || d.ctx != nil && d.ctx.Err() != nil {
			reason = "cancelled"
			s.Status = "cancelled"
		}
		s.Failure = &support.Failure{Code: reason, Stage: d.stage}
	}
	if d.effective != nil {
		o := d.effective
		roots := len(d.paths)
		if roots == 0 {
			roots = len(o.scopeRoots)
		}
		if roots == 0 {
			roots = 1
		}
		s.Settings = &support.Settings{Profile: o.profile, Threshold: o.threshold, MinTokens: int64(o.minTokens), MaxGroups: int64(o.maxGroups), MaxOccurrences: int64(o.maxOccurrences), MaxPairs: int64(o.maxPairs), MaxFileBytes: o.maxFileBytes, Workers: int64(o.workers), ComparisonDomain: o.comparisonDomain, SQLDialect: o.sqlDialect, FragmentSelection: o.fragmentSelection, Ranking: o.ranking, SameLanguageOnly: o.sameLanguageOnly, CrossLanguageOnly: o.crossLanguageOnly, EmbeddedSQL: o.embeddedSQL, StatementBlocks: o.statementBlocks, BlockStatements: int64(o.blockStatements), MaxBlocksPerFunction: int64(o.maxBlocksPerFunc), ExcludeGenerated: o.excludeGenerated, RespectIgnore: o.respectIgnore, FailOnMatch: o.failOnMatch, RequireCoverage: o.requireCoverage, MinFileCoverage: o.minFileCoverage, MaxZeroFragmentFiles: int64(o.maxZeroFiles), FailOnWarning: o.failOnWarning, FailOnParseDiagnostic: o.failOnDiagnostic, ScopeSelected: o.scope != "", RootCount: int64(roots), ExcludePatternCount: int64(len(o.excludes)), PriorityPathCount: int64(len(o.priorityPaths)), LanguagePairCount: int64(len(o.languagePairs)), BaselineEnabled: o.baselinePath != ""}
	}
	for _, phase := range []struct {
		name     string
		duration time.Duration
		observed bool
	}{{"discovery", d.discovery, d.discoveryObserved}, {"parse", d.timings.Parse, d.timings.ParseObserved}, {"compare", d.timings.Compare, d.timings.CompareObserved}} {
		if phase.observed {
			s.Phases = append(s.Phases, support.Phase{Name: phase.name, Milliseconds: phase.duration.Milliseconds()})
		}
	}
	if d.result == nil || d.result.SchemaVersion == 0 {
		return s
	}
	s.ReportAvailable = true
	r := d.result
	s.Counts = support.Counts{Files: int64(r.Files), Fragments: int64(r.Fragments), CandidatePairs: int64(r.CandidatePairs), LocationPairs: int64(r.TotalLocationPairs), MatchGroups: int64(r.TotalMatchGroups), Warnings: int64(len(r.Warnings)), ZeroFragmentFiles: int64(r.Coverage.ZeroFragmentFiles), GeneratedExcludedFiles: int64(r.Coverage.GeneratedExcluded), Truncated: r.Truncated}
	langs := map[string]support.LanguageCount{}
	for _, file := range r.FileCoverage {
		entry := langs[file.Language]
		entry.Language = file.Language
		entry.Files++
		entry.Fragments += int64(file.FragmentCount)
		langs[file.Language] = entry
		s.Counts.ParseDiagnostics += int64(file.ParseDiagnostics)
		switch diagnosticPathClass(file.Path) {
		case "story":
			s.Counts.StoryFiles++
			s.Counts.StoryFragments += int64(file.FragmentCount)
		case "test":
			s.Counts.TestFiles++
			s.Counts.TestFragments += int64(file.FragmentCount)
		}

	}
	ids := make([]string, 0, len(langs))
	for id := range langs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s.Languages = append(s.Languages, langs[id])
	}
	s.Counts.LeadingGroups = int64(min(25, len(r.Groups)))
	for _, group := range r.Groups[:int(s.Counts.LeadingGroups)] {
		found := false
		for _, profile := range group.Profiles {
			for _, occurrence := range profile.Occurrences {
				if diagnosticPathClass(occurrence.Location.Path) != "" {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			s.Counts.LeadingTestStoryGroups++
		}
	}
	return s
}

// Report paths can be absolute or parent-relative when scanning explicit roots.
// Ignore their ancestor directory names, which may not belong to the project.
func diagnosticPathClass(path string) string {
	path = pathutil.PortableSlash(path)
	if pathutil.IsRooted(path) || path == ".." || strings.HasPrefix(path, "../") {
		path = filepath.Base(path)
	}
	if strings.Contains(path, ".stories.") || strings.Contains("/"+path, "/stories/") || strings.Contains("/"+path, "/__stories__/") {
		return "story"
	}
	if pattern, _ := testPathConvention(path); pattern != "" {
		return "test"
	}
	return ""
}
