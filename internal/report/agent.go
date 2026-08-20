package report

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
)

const (
	maxAgentGroups   = 25
	maxAgentWarnings = 20
)

// Agent writes a bounded review summary intended for coding-agent context.
// The complete machine-readable report should be retained separately when
// this renderer is used for a provenance-sensitive review.
func Agent(writer io.Writer, value model.Report) error {
	focused := value.Configuration.Focus != nil
	visibleGroups := make([]model.MatchGroup, 0, min(len(value.Groups), maxAgentGroups))
	for _, group := range value.Groups {
		if focused && !group.Focused {
			continue
		}
		visibleGroups = append(visibleGroups, group)
		if len(visibleGroups) == maxAgentGroups {
			break
		}
	}

	relevantTotal := value.TotalMatchGroups
	label := "group(s)"
	if focused {
		relevantTotal = value.TotalFocusedMatchGroups
		label = "focused group(s)"
	}
	if _, err := fmt.Fprintf(
		writer,
		"Mori agent summary: %d %s; %d location pair(s); %d candidate pair(s) compared\n",
		relevantTotal,
		label,
		value.TotalLocationPairs,
		value.CandidatePairs,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(
		writer,
		"tool: mori %s; revision %s; source date %s; modified %t; %s/%s; normalization %d; report schema %d\n",
		terminalSafe(value.Tool.Version),
		terminalSafe(shortDigest(value.Tool.Revision)),
		terminalSafe(value.Tool.SourceDate),
		value.Tool.Modified,
		terminalSafe(value.Tool.GOOS),
		terminalSafe(value.Tool.GOARCH),
		value.Tool.NormalizationVersion,
		value.SchemaVersion,
	); err != nil {
		return err
	}

	if input := value.Configuration.Input; input != nil {
		if _, err := fmt.Fprintf(
			writer,
			"input: %s; HEAD %s; index %s; working tree %t; untracked %t\n",
			terminalSafe(input.Mode),
			terminalSafe(shortDigest(input.HeadCommit)),
			terminalSafe(shortDigest(input.IndexDigest)),
			input.WorkingTreeIncluded,
			input.UntrackedIncluded,
		); err != nil {
			return err
		}
	}
	if focus := value.Configuration.Focus; focus != nil {
		if _, err := fmt.Fprintf(
			writer,
			"focus: %d/%d supported non-deleted file(s) analyzed; %d focused identity/identities\n",
			focus.CoveredFocusFiles,
			focus.RequiredFocusFiles,
			value.TotalFocusedMatchGroups,
		); err != nil {
			return err
		}
		statuses := make(map[string]int)
		changedRanges := 0
		for _, evidence := range focus.PathEvidence {
			statuses[evidence.Status]++
			changedRanges += len(evidence.ChangedLines)
		}
		if _, err := fmt.Fprintf(
			writer,
			"focus evidence: %d changed line range(s); %d unsupported; %d deleted; %d excluded or undiscovered\n",
			changedRanges,
			statuses["unsupported"],
			statuses["deleted"],
			statuses["excluded_generated"]+statuses["skipped_resource"]+statuses["not_discovered"],
		); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(
		writer,
		"coverage: %d/%d supported file(s) analyzed; %d fragment file(s); %d zero-fragment file(s); %d warning(s); %d parse diagnostic(s)\n",
		value.Coverage.AnalyzedFiles,
		value.Coverage.SupportedFiles,
		value.Coverage.FragmentFiles,
		value.Coverage.ZeroFragmentFiles,
		value.Coverage.WarningCount,
		value.Coverage.ParseDiagnosticCount,
	); err != nil {
		return err
	}
	if receipt := value.Configuration.ReviewReceipt; receipt != nil {
		if _, err := fmt.Fprintf(
			writer,
			"review receipt: %s; schema %d; %d focused identity/identities; %s\n",
			terminalSafe(receipt.Status),
			receipt.SchemaVersion,
			receipt.FocusedMatchGroups,
			terminalSafe(shortDigest(receipt.Digest)),
		); err != nil {
			return err
		}
	}

	for index, group := range visibleGroups {
		if _, err := fmt.Fprintf(
			writer,
			"%d. %.1f%% %s; %d location pair(s); %s\n",
			index+1,
			group.Similarity*100,
			terminalSafe(group.ID),
			group.LocationPairs,
			terminalSafe(agentGroupLocations(group, value.Configuration.Focus)),
		); err != nil {
			return err
		}
	}
	if len(visibleGroups) < relevantTotal {
		if _, err := fmt.Fprintf(
			writer,
			"groups omitted from context: %d; inspect the complete JSON evidence\n",
			relevantTotal-len(visibleGroups),
		); err != nil {
			return err
		}
	}

	warningLimit := min(len(value.Warnings), maxAgentWarnings)
	for index := 0; index < warningLimit; index++ {
		warning := value.Warnings[index]
		kind := warning.Kind
		if kind == "" {
			kind = "other"
		}
		location := ""
		if warning.Path != "" {
			location = " " + warning.Path
		}
		if _, err := fmt.Fprintf(
			writer,
			"warning[%s]%s: %s\n",
			terminalSafe(kind),
			terminalSafe(location),
			terminalSafe(warning.Message),
		); err != nil {
			return err
		}
	}
	if warningLimit < len(value.Warnings) {
		if _, err := fmt.Fprintf(
			writer,
			"warnings omitted from context: %d; inspect the complete JSON evidence\n",
			len(value.Warnings)-warningLimit,
		); err != nil {
			return err
		}
	}
	if value.Truncated {
		if _, err := fmt.Fprintln(
			writer,
			"report retention was truncated; this summary is not complete evidence",
		); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(
		writer,
		"Scores identify structural review candidates; they do not prove behavioral equivalence.",
	)
	return err
}

func agentGroupLocations(group model.MatchGroup, focus *model.FocusConfig) string {
	if len(group.PathPairs) > 0 {
		selected := group.PathPairs[0]
		if focus != nil {
			for _, pair := range group.PathPairs {
				if agentFocusedLocation(pair.Left, focus) || agentFocusedLocation(pair.Right, focus) {
					selected = pair
					break
				}
			}
		}
		return formatAgentLocation(selected.Left) + " <-> " + formatAgentLocation(selected.Right)
	}

	type occurrence struct {
		profile  int
		location model.Location
	}
	all := make([]occurrence, 0)
	for profileIndex, profile := range group.Profiles {
		for _, summary := range profile.Occurrences {
			all = append(all, occurrence{profile: profileIndex, location: summary.Location})
		}
	}
	selected := make([]occurrence, 0, 2)
	if focus != nil {
		for _, candidate := range all {
			if agentFocusedLocation(candidate.location, focus) {
				selected = append(selected, candidate)
				break
			}
		}
	}
	for _, candidate := range all {
		if len(selected) > 0 && sameAgentLocation(candidate.location, selected[0].location) {
			continue
		}
		if len(selected) > 0 && candidate.profile == selected[0].profile {
			continue
		}
		selected = append(selected, candidate)
		break
	}
	if len(selected) == 1 {
		for _, candidate := range all {
			if !sameAgentLocation(candidate.location, selected[0].location) {
				selected = append(selected, candidate)
				break
			}
		}
	}
	if len(selected) == 0 && len(all) > 0 {
		selected = append(selected, all[0])
	}
	locations := make([]string, 0, len(selected))
	for _, candidate := range selected {
		locations = append(locations, formatAgentLocation(candidate.location))
		if len(locations) == 2 {
			break
		}
	}
	if len(locations) == 0 {
		return "representative locations unavailable"
	}
	return strings.Join(locations, " <-> ")
}

func formatAgentLocation(location model.Location) string {
	name := location.Name
	if name == "" {
		name = location.FragmentKind
	}
	line := fmt.Sprintf("%s:%d", location.Path, location.StartLine)
	if location.EndLine > location.StartLine {
		line = fmt.Sprintf("%s:%d-%d", location.Path, location.StartLine, location.EndLine)
	}
	return line + ":" + name
}

func agentFocusedLocation(location model.Location, focus *model.FocusConfig) bool {
	for _, evidence := range focus.PathEvidence {
		if filepath.ToSlash(filepath.Clean(evidence.Path)) != filepath.ToSlash(filepath.Clean(location.Path)) {
			continue
		}
		if len(evidence.ChangedLines) == 0 {
			return evidence.Status == "analyzed"
		}
		for _, interval := range evidence.ChangedLines {
			if location.StartLine <= interval.EndLine && location.EndLine >= interval.StartLine {
				return true
			}
		}
	}
	return false
}

func sameAgentLocation(left, right model.Location) bool {
	return left.Path == right.Path && left.StartLine == right.StartLine && left.EndLine == right.EndLine && left.Name == right.Name
}
