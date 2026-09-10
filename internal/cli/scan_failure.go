package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Cyberlane/mori/internal/analyzer"
	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/model"
)

// scanFailure is a separate artifact, deliberately not a model.Report. It has
// no groups, success status, receipt or baseline acceptance surface.
type scanFailure struct {
	RequestedPaths       []string              `json:"requested_paths"`
	Artifact             string                `json:"artifact"`
	FailureSchemaVersion int                   `json:"failure_schema_version"`
	Complete             bool                  `json:"complete"`
	Reason               string                `json:"reason"`
	Message              string                `json:"message"`
	Limit                int                   `json:"limit"`
	Compared             int                   `json:"compared_pairs"`
	Tool                 buildinfo.Info        `json:"tool"`
	Configuration        model.EffectiveConfig `json:"configuration"`
	Coverage             model.CoverageSummary `json:"coverage"`
	FileCoverage         []model.FileCoverage  `json:"file_coverage"`
	Warnings             []model.Warning       `json:"warnings"`
	Recovery             []string              `json:"recovery"`
}

func renderScanFailure(stdout, stderr io.Writer, result model.Report, options scanOptions, paths []string, scanErr error) int {
	var limit *analyzer.CandidateLimitError
	if !errors.As(scanErr, &limit) {
		return commandError(stderr, "scan", scanErr)
	}
	if options.redactPaths {
		redactReportPaths(&result)
	}
	roots := append([]string{}, paths...)
	if len(roots) == 0 {
		roots = append(roots, options.scopeRoots...)
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	if options.redactPaths {
		for i := range roots {
			roots[i] = fmt.Sprintf("<root-%03d>", i+1)
		}
	}
	failure := scanFailure{RequestedPaths: roots, Artifact: "mori-scan-failure", FailureSchemaVersion: 1, Complete: false, Reason: "candidate_pair_limit", Message: limit.Error(), Limit: limit.Limit, Compared: limit.Compared, Tool: result.Tool, Configuration: result.Configuration, Coverage: result.Coverage, FileCoverage: result.FileCoverage, Warnings: result.Warnings, Recovery: []string{"mori inspect --format json .", "Inspect the same project directory with setup --agent (configure --agent when .mori.json exists); review named scope suggestions.", "Retry scan with an explicitly chosen smaller root or --scope NAME; --max-groups does not reduce comparisons."}}
	encode := func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(failure)
	}
	if options.outputPath != "" {
		if err := writeJSONArtifact(options.outputPath, encode); err != nil {
			return commandError(stderr, "write incomplete failure evidence", err)
		}
		label := displayCLIPath(options.outputPath)
		if options.redactPaths {
			label = "<redacted output path>"
		}
		if _, err := fmt.Fprintf(stdout, "incomplete failure evidence: %s (no ranking or acceptance)\n", label); err != nil {
			return exitError
		}
	}
	if options.format == "json" {
		if err := encode(stdout); err != nil {
			return commandError(stderr, "write incomplete failure evidence", err)
		}
	}
	fmt.Fprintf(stderr, "mori: analysis incomplete: %v\nInspect scope with 'mori inspect --format json .' and choose smaller roots before retrying. No complete ranking or acceptance was produced.\n", scanErr)
	return exitError
}
