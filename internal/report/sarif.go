package report

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"

	"github.com/Cyberlane/mori/internal/model"
)

const (
	sarifVersion       = "2.1.0"
	sarifSchema        = "https://json.schemastore.org/sarif-2.1.0.json"
	similarityRuleID   = "MORI001"
	parseWarningRuleID = "MORI002"
)

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Invocations []sarifInvocation `json:"invocations"`
	Results     []sarifResult     `json:"results"`
	Properties  map[string]any    `json:"properties"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	ShortDescription sarifMessage `json:"shortDescription"`
	FullDescription  sarifMessage `json:"fullDescription"`
	HelpURI          string       `json:"helpUri"`
	DefaultConfig    sarifConfig  `json:"defaultConfiguration"`
}

type sarifConfig struct {
	Level string `json:"level"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool           `json:"executionSuccessful"`
	Properties          map[string]any `json:"properties"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations,omitempty"`
	RelatedLocations    []sarifLocation   `json:"relatedLocations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
	Properties          map[string]any    `json:"properties,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	ID               int                   `json:"id,omitempty"`
	Message          *sarifMessage         `json:"message,omitempty"`
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIF writes a deterministic SARIF 2.1.0 log. Unsuppressed match groups use
// stable rule MORI001; parser warnings use MORI002. Baseline-suppressed groups
// are intentionally omitted because the report no longer retains their source
// locations, while exact suppression counts remain in invocation properties.
func SARIF(writer io.Writer, report model.Report) error {
	results := make([]sarifResult, 0, len(report.Groups)+len(report.Warnings))
	for _, group := range report.Groups {
		if result, ok := sarifSimilarityResult(group); ok {
			results = append(results, result)
		}
	}
	for _, warning := range report.Warnings {
		results = append(results, sarifWarningResults(warning)...)
	}

	reportProperties := map[string]any{
		"moriReportSchemaVersion": report.SchemaVersion,
		"scanProfileDigest":       report.Configuration.ScanProfileDigest,
		"stdinPath":               report.Configuration.StdinPath,
	}
	if receipt := report.Configuration.ReviewReceipt; receipt != nil {
		reportProperties["reviewReceiptStatus"] = receipt.Status
		reportProperties["reviewReceiptDigest"] = receipt.Digest
		reportProperties["reviewReceiptFocusedMatchGroups"] = receipt.FocusedMatchGroups
	}
	log := sarifLog{
		Version: sarifVersion,
		Schema:  sarifSchema,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "Mori",
				Version:        report.Tool.Version,
				InformationURI: "https://github.com/Cyberlane/mori",
				Rules: []sarifRule{
					{
						ID: similarityRuleID, Name: "structural-similarity",
						ShortDescription: sarifMessage{Text: "Structurally similar source fragments"},
						FullDescription:  sarifMessage{Text: "Mori found normalized structural similarity that requires source review and does not prove semantic or behavioral equivalence."},
						HelpURI:          "https://github.com/Cyberlane/mori/blob/main/docs/scoring.md",
						DefaultConfig:    sarifConfig{Level: "note"},
					},
					{
						ID: parseWarningRuleID, Name: "incomplete-structural-analysis",
						ShortDescription: sarifMessage{Text: "Incomplete structural analysis"},
						FullDescription:  sarifMessage{Text: "Mori reported a parser, discovery, focus, baseline, or coverage warning that can make structural results incomplete."},
						HelpURI:          "https://github.com/Cyberlane/mori/blob/main/README.md#known-parser-limits",
						DefaultConfig:    sarifConfig{Level: "warning"},
					},
				},
			}},
			Invocations: []sarifInvocation{{
				ExecutionSuccessful: true,
				Properties: map[string]any{
					"candidatePairs":          report.CandidatePairs,
					"reportSchemaVersion":     report.SchemaVersion,
					"normalizationVersion":    report.Tool.NormalizationVersion,
					"retainedMatchGroups":     len(report.Groups),
					"totalMatchGroups":        report.TotalMatchGroups,
					"suppressedLocationPairs": report.SuppressedLocationPairs,
					"suppressedMatchGroups":   report.SuppressedMatchGroups,
					"truncated":               report.Truncated,
					"warningCount":            len(report.Warnings),
				},
			}},
			Results:    results,
			Properties: reportProperties,
		}},
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(log)
}

func sarifSimilarityResult(group model.MatchGroup) (sarifResult, bool) {
	locations := make([]model.FragmentSummary, 0)
	for _, profile := range group.Profiles {
		locations = append(locations, profile.Occurrences...)
	}
	if len(locations) == 0 {
		return sarifResult{}, false
	}
	primary := fragmentSARIFLocation(locations[0], 0)
	related := make([]sarifLocation, 0, len(locations)-1)
	for index, occurrence := range locations[1:] {
		related = append(related, fragmentSARIFLocation(occurrence, index+1))
	}
	label := "structural similarity"
	if group.Similarity == 1 {
		label = "normalized feature identity"
	}
	return sarifResult{
		RuleID: similarityRuleID,
		Level:  "note",
		Message: sarifMessage{Text: fmt.Sprintf(
			"%.1f%% %s across %d location pair(s); this does not prove semantic or behavioral equivalence",
			group.Similarity*100, label, group.LocationPairs,
		)},
		Locations:        []sarifLocation{primary},
		RelatedLocations: related,
		PartialFingerprints: map[string]string{
			"moriContentPairId/v1": group.ID,
		},
		Properties: map[string]any{
			"contentPairId":        group.ID,
			"focused":              group.Focused,
			"focusedOccurrences":   group.FocusedCount,
			"locationPairs":        group.LocationPairs,
			"reviewPriority":       group.ReviewPriority,
			"reviewSignals":        group.ReviewSignals,
			"similarity":           group.Similarity,
			"weightedIntersection": group.Evidence.Intersection,
			"weightedUnion":        group.Evidence.Union,
		},
	}, true
}

func fragmentSARIFLocation(fragment model.FragmentSummary, id int) sarifLocation {
	message := sarifMessage{Text: fmt.Sprintf(
		"%s %s (%s)", fragment.Location.FragmentKind, fragment.Location.Name, fragment.Location.Language,
	)}
	return sarifLocation{
		ID: id, Message: &message,
		PhysicalLocation: sarifPhysicalLocation{
			ArtifactLocation: sarifArtifactLocation{URI: sarifArtifactURI(fragment.Location.Path)},
			Region: sarifRegion{
				StartLine: fragment.Location.StartLine, StartColumn: 1,
				EndLine: fragment.Location.EndLine,
			},
		},
	}
}

func sarifWarningResults(warning model.Warning) []sarifResult {
	baseProperties := map[string]any{
		"kind":             warning.Kind,
		"language":         warning.Language,
		"skippedFragments": warning.SkippedFragments,
		"totalDiagnostics": warning.TotalDiagnostics,
	}
	if len(warning.Diagnostics) == 0 {
		result := sarifResult{
			RuleID: parseWarningRuleID, Level: "warning",
			Message: sarifMessage{Text: warning.Message}, Properties: baseProperties,
		}
		if warning.Path != "" {
			result.Locations = []sarifLocation{warningSARIFLocation(warning.Path, sarifRegion{StartLine: 1})}
		}
		return []sarifResult{result}
	}
	results := make([]sarifResult, 0, len(warning.Diagnostics))
	for _, diagnostic := range warning.Diagnostics {
		properties := make(map[string]any, len(baseProperties)+1)
		for key, value := range baseProperties {
			properties[key] = value
		}
		properties["nodeKind"] = diagnostic.NodeKind
		results = append(results, sarifResult{
			RuleID: parseWarningRuleID, Level: "warning",
			Message: sarifMessage{Text: warning.Message},
			Locations: []sarifLocation{warningSARIFLocation(warning.Path, sarifRegion{
				StartLine: diagnostic.StartLine, StartColumn: diagnostic.StartColumn,
				EndLine: diagnostic.EndLine, EndColumn: diagnostic.EndColumn,
			})},
			Properties: properties,
		})
	}
	return results
}

func warningSARIFLocation(path string, region sarifRegion) sarifLocation {
	return sarifLocation{PhysicalLocation: sarifPhysicalLocation{
		ArtifactLocation: sarifArtifactLocation{URI: sarifArtifactURI(path)},
		Region:           region,
	}}
}

func sarifArtifactURI(path string) string {
	slashed := filepath.ToSlash(path)
	if filepath.IsAbs(path) {
		if filepath.VolumeName(path) != "" && slashed[0] != '/' {
			slashed = "/" + slashed
		}
		return (&url.URL{Scheme: "file", Path: slashed}).String()
	}
	return (&url.URL{Path: slashed}).String()
}
