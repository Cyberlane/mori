package support

import (
	"errors"
	"math"
	"reflect"
	"regexp"
	"sort"
)

var errInvalid = errors.New("invalid support session")
var versionPattern = regexp.MustCompile(`^(unknown|dev|v?[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4}(\+dirty)?)$`)
var revisionPattern = regexp.MustCompile(`^(unknown|[a-f0-9]{7,40}(-dirty)?)$`)
var goPattern = regexp.MustCompile(`^(unknown|go[0-9]{1,3}\.[0-9]{1,3}(\.[0-9]{1,3})?((rc|beta)[0-9]{1,3})?)$`)
var osValues = []string{"unknown", "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows"}
var archValues = []string{"unknown", "386", "amd64", "arm", "arm64", "loong64", "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x", "wasm"}
var languageValues = []string{"bash", "c", "cpp", "csharp", "dart", "gdscript", "go", "java", "javascript", "lua", "kotlin", "luau", "php", "hack", "typescript", "tsx", "python", "powershell", "ruby", "rust", "swift", "zsh", "sql", "postgresql"}

func oneOf(s string, choices ...string) bool {
	for _, choice := range choices {
		if s == choice {
			return true
		}
	}
	return false
}

// SanitizeBuild retains official-format metadata while discarding arbitrary
// development build labels, which can contain local paths or other private text.
func SanitizeBuild(b Build) Build {
	if !versionPattern.MatchString(b.Version) {
		b.Version = "unknown"
	}
	if !revisionPattern.MatchString(b.Revision) {
		b.Revision = "unknown"
	}
	if !goPattern.MatchString(b.GoVersion) {
		b.GoVersion = "unknown"
	}
	if !oneOf(b.GOOS, osValues...) {
		b.GOOS = "unknown"
	}
	if !oneOf(b.GOARCH, archValues...) {
		b.GOARCH = "unknown"
	}
	return b
}

// Validate checks the complete export allowlist. No free-text field is accepted.
func Validate(s Session) error {
	if s.SchemaVersion != SchemaVersion || s.Tool != SanitizeBuild(s.Tool) {
		return errInvalid
	}
	for _, n := range []int{s.Tool.ReportSchema, s.Tool.NormalizationVersion, s.Tool.ConfigSchema, s.Tool.ContractSchema} {
		if n < 1 || n > 1000000 {
			return errInvalid
		}
	}
	if !oneOf(s.Status, "success", "error", "usage", "findings", "coverage", "upgrade", "cancelled") {
		return errInvalid
	}
	if s.Failure != nil {
		if !oneOf(s.Failure.Stage, "arguments", "validation", "configuration", "project", "baseline", "discovery", "analysis", "receipt", "output", "coverage") || !oneOf(s.Failure.Code, "error", "usage", "findings", "coverage", "upgrade", "cancelled", "resource-limit", "parse", "configuration", "invalid-arguments", "io", "unknown") {
			return errInvalid
		}
	}
	if s.Status == "success" && s.Failure != nil {
		return errInvalid
	}
	if !s.ReportAvailable && (s.Counts != (Counts{}) || len(s.Languages) != 0 || len(s.ReviewedFindings) != 0) {
		return errInvalid
	}
	if s.Counts.LeadingGroups > 25 || s.Counts.LeadingTestStoryGroups > s.Counts.LeadingGroups {
		return errInvalid
	}
	if !validIntegers(reflect.ValueOf(s.Counts), false) {
		return errInvalid
	}
	if s.Settings != nil {
		v := s.Settings
		if !oneOf(v.Profile, "", "default", "review", "explore", "sql", "strict", "custom") || !oneOf(v.ComparisonDomain, "", "code", "sql-query", "all") || !oneOf(v.SQLDialect, "", "generic", "postgresql") || !oneOf(v.FragmentSelection, "", "all", "production", "tests") || !oneOf(v.Ranking, "", "structural", "review") {
			return errInvalid
		}
		if !unit(v.Threshold) || !unit(v.MinFileCoverage) || !validIntegers(reflect.ValueOf(*v), true) || v.SameLanguageOnly && v.CrossLanguageOnly {
			return errInvalid
		}
	}
	if len(s.Phases) > 5 || len(s.Languages) > len(languageValues) || len(s.ReviewedFindings) > 25 {
		return errInvalid
	}
	seen := map[string]bool{}
	for _, phase := range s.Phases {
		if !oneOf(phase.Name, "total", "discovery", "parse", "compare", "analysis") || seen[phase.Name] || phase.Milliseconds < 0 || phase.Milliseconds > 604800000 {
			return errInvalid
		}
		seen[phase.Name] = true
	}
	seen = map[string]bool{}
	for _, lang := range s.Languages {
		if !oneOf(lang.Language, languageValues...) || seen[lang.Language] || lang.Files < 0 || lang.Files > maxInteger || lang.Fragments < 0 || lang.Fragments > maxInteger {
			return errInvalid
		}
		seen[lang.Language] = true
	}
	ranks := map[int]bool{}
	for _, f := range s.ReviewedFindings {
		if f.Rank < 1 || f.Rank > 1000000 || ranks[f.Rank] || !oneOf(f.Classification, "useful", "intentional", "false-positive", "uncertain") {
			return errInvalid
		}
		ranks[f.Rank] = true
	}
	return nil
}

const maxInteger = int64(math.MaxInt64)

func validIntegers(v reflect.Value, settings bool) bool {
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.Int64 {
			minimum := int64(0)
			if settings && v.Type().Field(i).Name == "MaxZeroFragmentFiles" {
				minimum = -1
			}
			if f.Int() < minimum || f.Int() > maxInteger {
				return false
			}
		}
	}
	return true
}
func unit(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 1 }
func canonical(s Session) Session {
	s.Phases = append([]Phase{}, s.Phases...)
	s.Languages = append([]LanguageCount{}, s.Languages...)
	s.ReviewedFindings = append([]ReviewedFinding{}, s.ReviewedFindings...)
	sort.Slice(s.Phases, func(i, j int) bool { return s.Phases[i].Name < s.Phases[j].Name })
	sort.Slice(s.Languages, func(i, j int) bool { return s.Languages[i].Language < s.Languages[j].Language })
	sort.Slice(s.ReviewedFindings, func(i, j int) bool { return s.ReviewedFindings[i].Rank < s.ReviewedFindings[j].Rank })
	return s
}
