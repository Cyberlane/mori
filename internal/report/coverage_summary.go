package report

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
)

// sourceCoverageSummary describes discovery separately from parser coverage.
// Counts include only the selected discovery surface, never the whole repository.
func sourceCoverageSummary(w io.Writer, value model.Report) error {
	if _, err := fmt.Fprintf(w,
		"coverage: %d/%d supported file(s) analyzed; %d fragment file(s); %d zero-fragment file(s); %d warning(s); %d parse diagnostic(s)\n",
		value.Coverage.AnalyzedFiles, value.Coverage.SupportedFiles,
		value.Coverage.FragmentFiles, value.Coverage.ZeroFragmentFiles,
		value.Coverage.WarningCount, value.Coverage.ParseDiagnosticCount,
	); err != nil {
		return err
	}
	languages := map[string]int{}
	for _, file := range value.FileCoverage {
		languages[file.Language]++
	}
	ids := make([]string, 0, len(languages))
	for id := range languages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%s=%d", terminalSafe(id), languages[id]))
	}
	if len(parts) > 0 {
		if _, err := fmt.Fprintf(w, "selected supported source: %s; generated exclusions %d\n", strings.Join(parts, ", "), value.Coverage.GeneratedExcluded); err != nil {
			return err
		}
	}
	excludedTests, excludedProduction := 0, 0
	for _, file := range value.FileCoverage {
		excludedTests += file.ExcludedTestFragments
		excludedProduction += file.ExcludedProductionFragments
	}
	if value.Configuration.FragmentSelection != "" && value.Configuration.FragmentSelection != "all" {
		if _, err := fmt.Fprintf(w, "fragment selection: %s; %d test and %d production/unclassified candidate(s) excluded; exclusions are not comparison coverage\n", terminalSafe(value.Configuration.FragmentSelection), excludedTests, excludedProduction); err != nil {
			return err
		}
	}
	unsupported := append([]model.UnsupportedExtension(nil), value.Coverage.UnsupportedExtensions...)
	sort.Slice(unsupported, func(i, j int) bool {
		if unsupported[i].FileCount != unsupported[j].FileCount {
			return unsupported[i].FileCount > unsupported[j].FileCount
		}
		return unsupported[i].Extension < unsupported[j].Extension
	})
	total := 0
	parts = nil
	for i, item := range unsupported {
		total += item.FileCount
		if i < 10 {
			parts = append(parts, fmt.Sprintf("%s=%d", terminalSafe(item.Extension), item.FileCount))
		}
	}
	if total > 0 {
		if _, err := fmt.Fprintf(w, "not examined: %d unsupported file(s) in selected discovery; %s", total, strings.Join(parts, ", ")); err != nil {
			return err
		}
		if len(unsupported) > 10 {
			if _, err := fmt.Fprintf(w, "; %d more extension(s) in JSON", len(unsupported)-10); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, "; supported-file coverage is not repository coverage"); err != nil {
			return err
		}
	}
	return scopeHint(w, value)
}
