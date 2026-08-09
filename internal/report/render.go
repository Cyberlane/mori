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

const maxTextCoverageFiles = 20

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
		"森 (mori): %d content-pair group(s) covering %d location pair(s) from %d fragment(s) in %d file(s)\n",
		report.TotalMatchGroups,
		report.TotalLocationPairs,
		report.Fragments,
		report.Files,
	); err != nil {
		return err
	}
	if report.Truncated {
		if _, err := fmt.Fprintf(
			writer,
			"showing the top %d group(s); increase --max-groups to see more identities\n",
			len(report.Groups),
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
	if report.SuppressedLocationPairs > 0 {
		if _, err := fmt.Fprintf(
			writer,
			"%d location pair(s) suppressed through %d baseline content identity/identities\n",
			report.SuppressedLocationPairs,
			report.SuppressedMatchGroups,
		); err != nil {
			return err
		}
	}
	if report.Configuration.Focus != nil {
		if _, err := fmt.Fprintf(
			writer,
			"%d focused group(s); %d focused file(s) discovered\n",
			report.TotalFocusedMatchGroups,
			report.Configuration.Focus.DiscoveredFocusFiles,
		); err != nil {
			return err
		}
	}
	if report.Configuration.ConfigPath != "" {
		if _, err := fmt.Fprintf(writer, "config %s\n", terminalSafe(report.Configuration.ConfigPath)); err != nil {
			return err
		}
	}
	if len(report.Configuration.IgnoreFiles) > 0 {
		if _, err := fmt.Fprintf(
			writer,
			"ignore rules: %s\n",
			terminalSafe(strings.Join(report.Configuration.IgnoreFiles, ", ")),
		); err != nil {
			return err
		}
	}
	if report.Configuration.Ranking == "review" {
		if _, err := fmt.Fprintln(
			writer,
			"ranking: explainable review signals before structural score",
		); err != nil {
			return err
		}
	}

	filesWithFragments := 0
	analyzedFiles := 0
	generatedAnalyzed := 0
	generatedExcluded := 0
	zeroFragmentFiles := make([]model.FileCoverage, 0)
	for _, coverage := range report.FileCoverage {
		if coverage.Status == "excluded_generated" {
			generatedExcluded++
			continue
		}
		analyzedFiles++
		if coverage.Generated {
			generatedAnalyzed++
		}
		if coverage.FragmentCount > 0 {
			filesWithFragments++
		} else {
			zeroFragmentFiles = append(zeroFragmentFiles, coverage)
		}
	}
	if analyzedFiles > 0 {
		if _, err := fmt.Fprintf(
			writer,
			"coverage: %d of %d analyzed file(s) produced comparison fragments at the current token floor\n",
			filesWithFragments,
			analyzedFiles,
		); err != nil {
			return err
		}
	}
	if generatedAnalyzed > 0 || generatedExcluded > 0 {
		if _, err := fmt.Fprintf(
			writer,
			"generated sources: %d analyzed, %d excluded\n",
			generatedAnalyzed,
			generatedExcluded,
		); err != nil {
			return err
		}
	}
	if len(zeroFragmentFiles) > 0 {
		if _, err := fmt.Fprintf(writer, "files without comparison fragments (%d):\n", len(zeroFragmentFiles)); err != nil {
			return err
		}
		for index, coverage := range zeroFragmentFiles {
			if index == maxTextCoverageFiles {
				if _, err := fmt.Fprintf(
					writer,
					"  - %d additional file(s) omitted; use --format json for the complete inventory\n",
					len(zeroFragmentFiles)-index,
				); err != nil {
					return err
				}
				break
			}
			if _, err := fmt.Fprintf(
				writer,
				"  - %s [%s]\n",
				terminalSafe(coverage.Path),
				terminalSafe(coverage.Language),
			); err != nil {
				return err
			}
		}
	}

	for index, group := range report.Groups {
		focusLabel := ""
		if group.Focused {
			focusLabel = fmt.Sprintf(" · focused (%d occurrence(s))", group.FocusedCount)
		}
		if _, err := fmt.Fprintf(
			writer,
			"\n%d. %.1f%% structural similarity · %d location pair(s)%s · identity %s\n",
			index+1,
			group.Similarity*100,
			group.LocationPairs,
			focusLabel,
			terminalSafe(group.ID),
		); err != nil {
			return err
		}
		if groupHasNestedBoundaries(group) {
			if _, err := fmt.Fprintln(
				writer,
				"   note: function bodies are excluded from this score and evaluated as separate comparison units",
			); err != nil {
				return err
			}
		}
		if group.LiteralEvidence != nil {
			evidence := group.LiteralEvidence
			if evidence.PairsWithDifferences == 0 {
				if _, err := fmt.Fprintf(
					writer,
					"   literal evidence: %d location pair(s) compared with no positional differences; values omitted\n",
					evidence.ComparedPairs,
				); err != nil {
					return err
				}
			} else if _, err := fmt.Fprintf(
				writer,
				"   literal evidence: %d of %d location pair(s) differ at up to %d position(s); values omitted\n",
				evidence.PairsWithDifferences,
				evidence.ComparedPairs,
				evidence.MaxDifferingPositions,
			); err != nil {
				return err
			}
		}
		if report.Configuration.Ranking == "review" {
			signals := "none"
			if len(group.ReviewSignals) > 0 {
				signals = strings.Join(group.ReviewSignals, ", ")
			}
			if _, err := fmt.Fprintf(
				writer,
				"   review priority %d · %s\n",
				group.ReviewPriority,
				terminalSafe(signals),
			); err != nil {
				return err
			}
		}
		for profileIndex, profile := range group.Profiles {
			label := string(rune('A' + profileIndex))
			if len(group.Profiles) == 1 {
				label = "P"
			}
			if _, err := fmt.Fprintf(
				writer,
				"   %s  fingerprint %s · %d occurrence(s)\n",
				label,
				terminalSafe(profile.Fingerprint),
				profile.OccurrenceCount,
			); err != nil {
				return err
			}
			for _, occurrence := range profile.Occurrences {
				if _, err := fmt.Fprintf(writer, "      - %s\n", formatFragment(occurrence)); err != nil {
					return err
				}
			}
			if profile.OccurrenceCount > len(profile.Occurrences) {
				if _, err := fmt.Fprintf(
					writer,
					"      - %d additional occurrence(s) omitted; increase --max-occurrences\n",
					profile.OccurrenceCount-len(profile.Occurrences),
				); err != nil {
					return err
				}
			}
		}
		if len(group.ShapeSummary) > 0 {
			if _, err := fmt.Fprintf(
				writer,
				"      shared shape: %s\n",
				strings.Join(group.ShapeSummary, ", "),
			); err != nil {
				return err
			}
		}
		if len(group.SharedFeatures) > 0 {
			features := make([]string, 0, len(group.SharedFeatures))
			for _, feature := range group.SharedFeatures {
				features = append(features, fmt.Sprintf("%s ×%d", feature.Feature, feature.Count))
			}
			if _, err := fmt.Fprintf(writer, "      raw features: %s\n", strings.Join(features, ", ")); err != nil {
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
			if _, err := fmt.Fprintf(writer, "  - %s%s\n", prefix, terminalSafe(warning.Message)); err != nil {
				return err
			}
			for _, diagnostic := range warning.Diagnostics {
				if _, err := fmt.Fprintf(
					writer,
					"      %s at %d:%d-%d:%d\n",
					terminalSafe(diagnostic.NodeKind),
					diagnostic.StartLine,
					diagnostic.StartColumn,
					diagnostic.EndLine,
					diagnostic.EndColumn,
				); err != nil {
					return err
				}
			}
			if warning.TotalDiagnostics > len(warning.Diagnostics) {
				if _, err := fmt.Fprintf(
					writer,
					"      %d additional parse diagnostic(s) omitted\n",
					warning.TotalDiagnostics-len(warning.Diagnostics),
				); err != nil {
					return err
				}
			}
			if warning.SkippedFragments > 0 {
				if _, err := fmt.Fprintf(
					writer,
					"      %d comparison fragment(s) containing parse errors skipped\n",
					warning.SkippedFragments,
				); err != nil {
					return err
				}
			}
		}
	}

	_, err := fmt.Fprintln(
		writer,
		"\nScores identify review candidates; they do not prove behavioral equivalence.",
	)
	return err
}

func groupHasNestedBoundaries(group model.MatchGroup) bool {
	for _, profile := range group.Profiles {
		for _, occurrence := range profile.Occurrences {
			if occurrence.NestedCount > 0 {
				return true
			}
		}
	}
	return false
}

func formatFragment(fragment model.FragmentSummary) string {
	location := fragment.Location
	lines := fmt.Sprintf("%d-%d", location.StartLine, location.EndLine)
	if location.StartLine == location.EndLine {
		lines = fmt.Sprintf("%d", location.StartLine)
	}
	nested := ""
	if fragment.NestedCount > 0 {
		if location.FragmentKind == "script" {
			nested = fmt.Sprintf(
				"; top-level body only, %d function(s) evaluated separately",
				fragment.NestedCount,
			)
		} else {
			nested = fmt.Sprintf(
				"; outer body only, %d nested function(s) evaluated separately",
				fragment.NestedCount,
			)
		}
	} else if fragment.NestingDepth > 0 {
		nested = fmt.Sprintf("; nesting depth %d", fragment.NestingDepth)
	}
	languageLabel := location.Language
	if location.LanguageFamily != "" && location.LanguageFamily != location.Language {
		languageLabel += "/" + location.LanguageFamily
	}
	return fmt.Sprintf(
		"%s:%s  [%s] %s  (%d tokens%s)",
		terminalSafe(location.Path),
		lines,
		terminalSafe(languageLabel),
		terminalSafe(location.Name),
		fragment.TokenCount,
		nested,
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
