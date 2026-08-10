package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
)

// Compact writes a bounded one-line-per-group review summary.
func Compact(writer io.Writer, report model.Report) error {
	if _, err := fmt.Fprintf(
		writer,
		"森: %d group(s), %d location pair(s), %d fragment(s), %d file(s); threshold %.1f%%\n",
		report.TotalMatchGroups, report.TotalLocationPairs, report.Fragments, report.Files,
		report.Threshold*100,
	); err != nil {
		return err
	}
	if report.Configuration.Input != nil && report.Configuration.Input.Mode == "git-index" {
		if _, err := fmt.Fprintf(writer, "input: git-index %s\n", terminalSafe(shortDigest(report.Configuration.Input.IndexDigest))); err != nil {
			return err
		}
	}
	if focus := report.Configuration.Focus; focus != nil {
		if _, err := fmt.Fprintf(
			writer, "focus: %d/%d supported non-deleted file(s) analyzed; %d focused group(s)\n",
			focus.CoveredFocusFiles, focus.RequiredFocusFiles, report.TotalFocusedMatchGroups,
		); err != nil {
			return err
		}
	}
	if receipt := report.Configuration.ReviewReceipt; receipt != nil {
		if _, err := fmt.Fprintf(
			writer, "review receipt: %s; %d focused group(s); %s\n",
			terminalSafe(receipt.Status), receipt.FocusedMatchGroups, terminalSafe(shortDigest(receipt.Digest)),
		); err != nil {
			return err
		}
	}
	for index, group := range report.Groups {
		locations := compactGroupLocations(group)
		focus := ""
		if group.Focused {
			focus = " focused"
		}
		if _, err := fmt.Fprintf(
			writer, "%d %.1f%% %s%s pairs=%d %s\n",
			index+1, group.Similarity*100, terminalSafe(group.ID), focus,
			group.LocationPairs, terminalSafe(locations),
		); err != nil {
			return err
		}
	}
	if report.Truncated {
		if _, err := fmt.Fprintf(writer, "truncated: showing %d of %d group(s)\n", len(report.Groups), report.TotalMatchGroups); err != nil {
			return err
		}
	}
	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintf(writer, "warnings: %d; use --format json for structured details\n", len(report.Warnings)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer, "Scores identify review candidates; they do not prove behavioral equivalence.")
	return err
}

func compactGroupLocations(group model.MatchGroup) string {
	values := make([]string, 0, len(group.Profiles))
	for _, profile := range group.Profiles {
		if len(profile.Occurrences) == 0 {
			continue
		}
		location := profile.Occurrences[0].Location
		name := location.Name
		if name == "" {
			name = location.FragmentKind
		}
		values = append(values, fmt.Sprintf("%s:%d:%s", location.Path, location.StartLine, name))
	}
	if len(values) == 0 {
		return "locations unavailable"
	}
	return strings.Join(values, " <-> ")
}

func shortDigest(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
