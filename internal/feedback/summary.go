package feedback

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// MaxBundles bounds one offline summary. Exports are deliberately not assigned
// project identifiers, so exports cannot be counted as independent projects.
const MaxBundles = 64

// ReadBundle accepts only the minimized sharing contract, never scan reports.
func ReadBundle(r io.Reader) (Bundle, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil || len(raw) > maxBytes {
		return Bundle{}, errors.New("feedback export exceeds limit or cannot be read")
	}
	var bundle Bundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return Bundle{}, errors.New("invalid feedback export")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || bundle.SchemaVersion != SchemaVersion || bundle.Samples == nil || len(bundle.Samples) > MaxRecords {
		return Bundle{}, errors.New("invalid feedback export contract")
	}
	// Zero-value booleans must not make missing required fields look valid.
	var fields struct {
		Samples []map[string]json.RawMessage `json:"samples"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Bundle{}, errors.New("invalid feedback export")
	}
	for i, sample := range bundle.Samples {
		for _, value := range fields.Samples[i] {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return Bundle{}, errors.New("invalid null feedback field")
			}
		}
		if _, present := fields.Samples[i]["truncated"]; !present || string(fields.Samples[i]["truncated"]) == "null" || !validSample(sample) {
			return Bundle{}, errors.New("invalid feedback sample")
		}
	}
	return bundle, nil
}

// Summarize writes deterministic counts, not estimated timings or inferred
// usefulness. Identical bundles are counted and disclosed; overlap is not identifiable.
func Summarize(w io.Writer, bundles []Bundle) error {
	if len(bundles) == 0 || len(bundles) > MaxBundles {
		return errors.New("summary requires 1 to 64 exports")
	}
	seen := make(map[string]bool)
	counts := make(map[string]int)
	total, annotated, efforts, ranked, truncated, identical := 0, 0, 0, 0, 0, 0
	for _, bundle := range bundles {
		if bundle.SchemaVersion != SchemaVersion || bundle.Samples == nil || len(bundle.Samples) > MaxRecords {
			return errors.New("invalid feedback export contract")
		}
		canonical, err := json.Marshal(bundle)
		if err != nil {
			return errors.New("invalid feedback export")
		}
		if seen[string(canonical)] {
			identical++
		}
		seen[string(canonical)] = true
		for _, sample := range bundle.Samples {
			if !validSample(sample) {
				return errors.New("invalid feedback sample")
			}
			total++
			if sample.Truncated {
				truncated++
			}
			counts["outcome="+sample.Outcome]++
			counts["analysis="+sample.Analysis]++
			counts["findings scope="+sample.FindingsScope+" count="+sample.Findings]++
			counts["timing version="+sample.ToolVersion+" policy="+sample.Policy+" cache="+sample.Cache+" files="+sample.Files+" duration="+sample.Duration]++
			for language, count := range sample.Languages {
				if count != "0" {
					counts["language present="+language]++
				}
			}
			for category, count := range sample.WarningCategories {
				if count != "0" {
					counts["warning present="+category]++
				}
			}
			if sample.Classification != "" {
				annotated++
				counts["selected finding="+sample.Classification]++
				if sample.ReviewedRank != "" {
					ranked++
					counts["selected rank="+sample.ReviewedRank+" classification="+sample.Classification]++
				}
				if sample.Effort != "" {
					efforts++
					counts["review effort policy="+sample.Policy+" effort="+sample.Effort]++
				}
			}
		}
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "Offline feedback summary\nExports: %d (not a project count)\nSamples: %d\nSelected finding annotations: %d\nAnnotations with rank: %d\nSamples with self-reported review effort: %d\nTruncated samples: %d\n", len(bundles), total, annotated, ranked, efforts, truncated)
	if identical > 0 {
		fmt.Fprintf(&out, "Identical export contents: %d additional exports (counted; verify independent collection windows)\n", identical)
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&out, "%s: %d\n", key, counts[key])
	}
	fmt.Fprintln(&out, "Selected annotations are not all findings. Missing annotations are unknown. Language/warning presence counts can overlap.\nExports can overlap; distinct projects and repeated scans cannot be identified. Compare timing strata only for matched workloads; coarse buckets do not establish speedups or causation.\nNo ranking changes, enrollment, or network submission performed.")
	_, err := w.Write(out.Bytes())
	return err
}
