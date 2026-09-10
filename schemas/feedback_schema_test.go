package schemas_test

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/feedback"
	"github.com/Cyberlane/mori/internal/model"
)

func TestFeedbackSchemaAllowlistMatchesModel(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("mori-feedback-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	props := object(t, schema["properties"], "properties")
	if object(t, props["schema_version"], "version")["const"] != float64(feedback.SchemaVersion) {
		t.Fatal("feedback version mismatch")
	}
	samples := object(t, props["samples"], "samples")
	if samples["maxItems"] != float64(feedback.MaxRecords) {
		t.Fatal("retention mismatch")
	}
	sample := object(t, samples["items"], "sample")
	if sample["additionalProperties"] != false {
		t.Fatal("feedback must reject unknown fields")
	}
	checkSchemaFields(t, sample, reflect.TypeOf(feedback.Sample{}))
}

func TestReviewOutcomeSchemaMatchesModel(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("mori-report-v21.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	props := object(t, schema["properties"], "properties")
	review := object(t, props["review"], "review")
	checkSchemaFields(t, review, reflect.TypeOf(model.ReviewOutcome{}))
}

func checkSchemaFields(t *testing.T, schema map[string]any, typ reflect.Type) {
	t.Helper()
	props := object(t, schema["properties"], "properties")
	var want, required []string
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")
		want = append(want, tag[0])
		if len(tag) == 1 {
			required = append(required, tag[0])
		}
	}
	got := make([]string, 0, len(props))
	for key := range props {
		got = append(got, key)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("properties: %v want %v", got, want)
	}
	gotRequired := stringSlice(t, schema["required"], "required")
	sort.Strings(gotRequired)
	sort.Strings(required)
	if !reflect.DeepEqual(gotRequired, required) {
		t.Fatalf("required: %v want %v", gotRequired, required)
	}
}
