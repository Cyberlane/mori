package analyzer

import "fmt"

// CandidateLimitError indicates that parsing completed but comparison did not.
// The accompanying report contains inventory, not a complete ranking.
type CandidateLimitError struct{ Limit, Compared int }

func (e *CandidateLimitError) Error() string {
	return fmt.Sprintf("candidate pair limit of %d reached after %d comparisons; choose a smaller source root or named scope, add explicit --exclude patterns, or raise --min-tokens; --max-groups only limits display, not computation", e.Limit, e.Compared)
}
