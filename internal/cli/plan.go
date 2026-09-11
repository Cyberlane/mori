package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
)

// A plan is deliberately not a scan report or evidence for a baseline/receipt.
// Candidate counts use the current parser and selection rules but no scores.
type scanPlan struct {
	Artifact          string                `json:"artifact"`
	SchemaVersion     int                   `json:"schema_version"`
	AnalysisPerformed bool                  `json:"analysis_performed"`
	RequestedPaths    []string              `json:"requested_paths"`
	Configuration     model.EffectiveConfig `json:"configuration"`
	Files             int                   `json:"files"`
	Fragments         int                   `json:"fragments"`
	CandidatePairs    int                   `json:"candidate_pairs"`
	ExceedsPairLimit  bool                  `json:"exceeds_pair_limit"`
	Coverage          model.CoverageSummary `json:"coverage"`
	Warnings          []model.Warning       `json:"warnings"`
	Packages          []planPackage         `json:"packages"`
}

type planPackage struct {
	Root           string `json:"root"`
	Files          int    `json:"files"`
	Fragments      int    `json:"fragments"`
	PairUpperBound int64  `json:"pair_upper_bound"`
}

func runPlan(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("plan", args, stderr, "plan")
	if !ok {
		return code
	}
	if options.format != "text" && options.format != "json" {
		return usageError(stderr, "plan supports --format text or json")
	}
	if options.focusedOnly || options.changedSince != "" || len(options.changedWorktrees) > 0 || len(options.focusPaths) > 0 {
		return usageError(stderr, "plan does not support focused scans")
	}
	if code := enforceProjectCompatibility(ctx, options, paths, stderr); code != exitSuccess {
		return code
	}
	options.estimateOnly = true
	result, err := executeScan(ctx, paths, options, nil, nil)
	if err != nil {
		return commandError(stderr, "plan", err)
	}
	roots := append([]string{}, paths...)
	if len(roots) == 0 {
		roots = append(roots, options.scopeRoots...)
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	packages := planPackages(options, roots, result.FileCoverage)
	if options.redactPaths {
		redactReportPaths(&result)
		for i := range roots {
			roots[i] = fmt.Sprintf("<root-%03d>", i+1)
		}
		for i := range packages {
			packages[i].Root = fmt.Sprintf("<package-%03d>", i+1)
		}
	}
	plan := scanPlan{Artifact: "mori-scan-plan", SchemaVersion: 1,
		RequestedPaths: roots, Configuration: result.Configuration,
		Files: result.Files, Fragments: result.Fragments, CandidatePairs: result.CandidatePairs,
		ExceedsPairLimit: options.maxPairs > 0 && result.CandidatePairs > options.maxPairs,
		Coverage:         result.Coverage, Warnings: append([]model.Warning{}, result.Warnings...), Packages: packages,
	}
	if options.format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(plan); err != nil {
			return exitError
		}
	} else if err := writeScanPlan(stdout, plan); err != nil {
		return exitError
	}
	// Coverage policy remains meaningful after parsing. Findings policy does not:
	// no pair was scored, and a plan never certifies a clean review.
	return enforceCoveragePolicies(stderr, options, result)
}

func writeScanPlan(w io.Writer, plan scanPlan) error {
	if _, err := fmt.Fprintf(w, "Mori scan plan: %d file(s), %d fragment(s), %d candidate pair(s)\nNo similarities scored. Baselines and finding policies are not evaluated.\nPair limit: %d; exceeds limit: %t\nCoverage: %d/%d supported file(s) analyzed; %d warning(s); %d parse diagnostic(s)\n",
		plan.Files, plan.Fragments, plan.CandidatePairs, plan.Configuration.MaxPairs, plan.ExceedsPairLimit,
		plan.Coverage.AnalyzedFiles, plan.Coverage.SupportedFiles, plan.Coverage.WarningCount, plan.Coverage.ParseDiagnosticCount); err != nil {
		return err
	}
	for _, p := range plan.Packages {
		if _, err := fmt.Fprintf(w, "  %s: %d file(s), %d fragment(s), at most %d within-root pair(s)\n", terminalPlanPath(p.Root), p.Files, p.Fragments, p.PairUpperBound); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "Root counts describe selected files, not a separate scan. Upper bounds precede language, size and overlap filtering. Separate package scans omit cross-package matches. Choose explicit roots or configure a named scope; a name alone does not narrow work. A pair limit of 0 is unlimited. Source changes require a new plan.")
	return err
}

func terminalPlanPath(path string) string { return fmt.Sprintf("%q", path) }

func planPackages(options scanOptions, roots []string, files []model.FileCoverage) []planPackage {
	base := "."
	if options.configPath != "" {
		base = filepath.Dir(options.configPath)
	} else if len(roots) == 1 {
		base = roots[0]
		if info, err := os.Stat(base); err == nil && !info.IsDir() {
			base = filepath.Dir(base)
		}
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return []planPackage{}
	}
	groups := map[string]*planPackage{}
	for _, file := range files {
		if file.Status != "analyzed" {
			continue
		}
		absolute, err := filepath.Abs(file.Path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, absolute)
		if err != nil {
			continue
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		root := "."
		if parts[0] == ".." {
			root = filepath.Dir(absolute)
		} else if len(parts) >= 3 && (parts[0] == "apps" || parts[0] == "packages") {
			root = filepath.Join(base, parts[0], parts[1])
		} else if len(parts) >= 2 {
			root = filepath.Join(base, parts[0])
		} else {
			root = base
		}
		entry := groups[root]
		if entry == nil {
			entry = &planPackage{Root: displayCLIPath(root)}
			groups[root] = entry
		}
		entry.Files++
		entry.Fragments += file.FragmentCount
	}
	result := make([]planPackage, 0, len(groups))
	for _, entry := range groups {
		n := int64(entry.Fragments)
		entry.PairUpperBound = n * (n - 1) / 2
		result = append(result, *entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Root < result[j].Root })
	return result
}
