// Package report renders stable machine-readable and human-readable reports.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/Cyberlane/mori/internal/model"
)

// JSON writes an indented JSON report.
func JSON(writer io.Writer, report model.Report) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// Text writes a compact report intended for terminals and CI logs.
func Text(writer io.Writer, report model.Report) error {
	if _, err := fmt.Fprintf(
		writer,
		"森 (mori): %d similarity candidate(s) from %d fragment(s) in %d file(s)\n",
		report.TotalMatches,
		report.Fragments,
		report.Files,
	); err != nil {
		return err
	}
	if report.Truncated {
		if _, err := fmt.Fprintf(
			writer,
			"showing the top %d match(es); increase --max-matches to see more\n",
			len(report.Matches),
		); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(
		writer,
		"threshold %.1f%% · %d candidate pair(s) compared\n",
		report.Threshold*100,
		report.CandidatePairs,
	); err != nil {
		return err
	}

	for index, match := range report.Matches {
		if _, err := fmt.Fprintf(
			writer,
			"\n%d. %.1f%% structural similarity\n",
			index+1,
			match.Similarity*100,
		); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "   A  %s\n", formatFragment(match.Left)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "   B  %s\n", formatFragment(match.Right)); err != nil {
			return err
		}
		if len(match.SharedFeatures) > 0 {
			features := make([]string, 0, len(match.SharedFeatures))
			for _, feature := range match.SharedFeatures {
				features = append(features, fmt.Sprintf("%s ×%d", feature.Feature, feature.Count))
			}
			if _, err := fmt.Fprintf(writer, "      shared: %s\n", strings.Join(features, ", ")); err != nil {
				return err
			}
		}
	}

	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintf(writer, "\nWarnings (%d):\n", len(report.Warnings)); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			prefix := ""
			if warning.Path != "" {
				prefix = terminalSafe(warning.Path) + ": "
			}
			if _, err := fmt.Fprintf(
				writer,
				"  - %s%s\n",
				prefix,
				terminalSafe(warning.Message),
			); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(
		writer,
		"\nScores identify review candidates; they do not prove behavioral equivalence.",
	)
	return err
}

func formatFragment(fragment model.FragmentSummary) string {
	location := fragment.Location
	lines := fmt.Sprintf("%d-%d", location.StartLine, location.EndLine)
	if location.StartLine == location.EndLine {
		lines = fmt.Sprintf("%d", location.StartLine)
	}
	return fmt.Sprintf(
		"%s:%s  [%s] %s  (%d tokens)",
		terminalSafe(location.Path),
		lines,
		terminalSafe(location.Language),
		terminalSafe(location.Name),
		fragment.TokenCount,
	)
}

func terminalSafe(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
			if character <= 0xffff {
				fmt.Fprintf(&builder, "\\u%04X", character)
			} else {
				fmt.Fprintf(&builder, "\\U%08X", character)
			}
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String()
}
