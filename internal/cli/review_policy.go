package cli

import (
	"io"

	"github.com/Cyberlane/mori/internal/model"
)

func stagedReviewOutcome(options scanOptions, result model.Report) *model.ReviewOutcome {
	coverageMet := enforceCoveragePolicies(io.Discard, options, result) == exitSuccess
	analysis := "complete"
	if result.Coverage.SupportedFiles == 0 || result.Fragments == 0 {
		analysis = "no-comparable-source"
	} else if !coverageMet || len(result.Warnings) > 0 || result.Truncated ||
		result.Coverage.ZeroFragmentFiles > 0 || result.Coverage.ParseDiagnosticCount > 0 ||
		result.Coverage.AnalyzedFiles < result.Coverage.SupportedFiles {
		analysis = "incomplete"
	}
	acknowledged := result.Configuration.ReviewReceipt != nil
	status := "passed"
	if !coverageMet || (options.reviewPolicy == "strict" && result.TotalFocusedMatchGroups > 0 && !acknowledged) {
		status = "blocked"
	}
	return &model.ReviewOutcome{
		Policy: options.reviewPolicy, Status: status, Analysis: analysis,
		CoveragePolicyMet: coverageMet, Findings: result.TotalFocusedMatchGroups, Acknowledged: acknowledged,
	}
}
