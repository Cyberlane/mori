package feedback

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Cyberlane/mori/internal/model"
)

func TestSummarySeparatesReviewDenominatorsAndTiming(t *testing.T) {
	sample := Measure(model.Report{TotalMatchGroups: 1}, time.Second, "completed", "hit")
	sample.Languages = map[string]string{"sql": "0", "go": "1-9"}
	classified := sample
	classified.Classification = "intentional"
	classified.ReviewedRank = "6-10"
	classified.Effort = "1-5m"
	bundles := []Bundle{{SchemaVersion: 1, Samples: []Sample{sample, classified}}}
	var out bytes.Buffer
	if err := Summarize(&out, bundles); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Samples: 2", "Selected finding annotations: 1", "Annotations with rank: 1", "Samples with self-reported review effort: 1", "timing version=dev policy=scan cache=hit files=0 duration=1-9s: 2", "selected rank=6-10 classification=intentional: 1", "Missing annotations are unknown"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "language present=sql") || !strings.Contains(out.String(), "language present=go: 2") {
		t.Fatal("incorrect language presence counts")
	}
	var again bytes.Buffer
	if err := Summarize(&again, bundles); err != nil || again.String() != out.String() {
		t.Fatal("nondeterministic summary", err)
	}
	again.Reset()
	if err := Summarize(&again, append(bundles, bundles...)); err != nil || !strings.Contains(again.String(), "Identical export contents: 1") || !strings.Contains(again.String(), "Samples: 4") {
		t.Fatal("identical exports not disclosed and counted", err)
	}
}

func TestReadBundleRejectsReportsAndUnknownData(t *testing.T) {
	sample := Measure(model.Report{}, 0, "completed")
	raw, _ := json.Marshal(Bundle{SchemaVersion: 1, Samples: []Sample{sample}})
	if _, err := ReadBundle(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		`{"schema_version":21,"samples":[]}`,
		`{"schema_version":1,"samples":null}`,
		`{"schema_version":1,"samples":[],"source":"private"}`,
		string(raw) + ` {}`,
		strings.Replace(string(raw), `"findings":"0"`, `"findings":"0","classification":"useful"`, 1),
		strings.Replace(string(raw), `"truncated":false`, `"truncated":null`, 1),
		strings.Replace(string(raw), `,"truncated":false`, "", 1),
		strings.Replace(string(raw), `"outcome":"completed"`, `"outcome":"private-source"`, 1),
		strings.Replace(string(raw), `"cache":"bypassed"`, `"cache":"bypassed","source":"private"`, 1),
		strings.Repeat(" ", maxBytes+1),
	} {
		if _, err := ReadBundle(strings.NewReader(invalid)); err == nil {
			t.Fatalf("accepted invalid %.100s", invalid)
		}
	}
}

func TestSummaryInputBounds(t *testing.T) {
	for _, bundles := range [][]Bundle{nil, make([]Bundle, MaxBundles+1), {{SchemaVersion: 1, Samples: []Sample{{}}}}} {
		var out bytes.Buffer
		if err := Summarize(&out, bundles); err == nil || out.Len() != 0 {
			t.Fatal("invalid input produced output")
		}
	}
}
