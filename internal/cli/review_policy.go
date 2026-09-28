package cli

import (
	"io"

	"github.com/Cyberlane/mori/internal/model"
)

func stagedReviewOutcome(options scanOptions, result model.Report) *model.ReviewOutcome {
	coverageMet := enforceCoveragePolicies(io.Discard, options, result) == exitSuccess
	analysis := "complete"
	var reasons []string
	if result.Coverage.SupportedFiles == 0 || result.Fragments == 0 {
		analysis = "no-comparable-source"
	} else {
		reasons = incompleteAnalysisReasons(coverageMet, result)
		if len(reasons) > 0 {
			analysis = "incomplete"
		}
	}
	acknowledged := result.Configuration.ReviewReceipt != nil
	status := "passed"
	if !coverageMet || (options.reviewPolicy == "strict" && result.TotalFocusedMatchGroups > 0 && !acknowledged) {
		status = "blocked"
	}
	return &model.ReviewOutcome{
		Policy: options.reviewPolicy, Status: status, Analysis: analysis, AnalysisReasons: reasons,
		CoveragePolicyMet: coverageMet, Findings: result.TotalFocusedMatchGroups, Acknowledged: acknowledged,
	}
}

// incompleteAnalysisReasons returns stable, sorted reason codes for evidence
// gaps. Files that were analyzed but contain no comparable fragments are not
// gaps by themselves: invalid, opaque, or resource-limited input always carries
// a warning, and the remaining zero-fragment reasons (no boundaries, below the
// token floor, fragment selection) are expected. Deliberate generated-source
// exclusions are policy, not missing coverage. Projects that must fail on
// either use max_zero_fragment_files or min_file_coverage, which surface here as
// coverage_policy_unmet.
func incompleteAnalysisReasons(coverageMet bool, result model.Report) []string {
	var reasons []string
	if !coverageMet {
		reasons = append(reasons, "coverage_policy_unmet")
	}
	if result.Coverage.ParseDiagnosticCount > 0 {
		reasons = append(reasons, "parse_diagnostics")
	}
	if result.Truncated {
		reasons = append(reasons, "truncated")
	}
	if result.Coverage.AnalyzedFiles+result.Coverage.GeneratedExcluded < result.Coverage.SupportedFiles {
		reasons = append(reasons, "unanalyzed_files")
	}
	if len(result.Warnings) > 0 {
		reasons = append(reasons, "warnings")
	}
	return reasons
}
