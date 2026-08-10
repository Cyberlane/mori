package schemas_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestCurrentReportSchemaMatchesPublicModel(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve schema test path")
	}
	schemaPath := filepath.Join(filepath.Dir(currentFile), "mori-report-v19.schema.json")
	content, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(content, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	properties := object(t, schema["properties"], "properties")
	version := object(t, properties["schema_version"], "schema_version")
	if got := int(version["const"].(float64)); got != model.SchemaVersion {
		t.Fatalf("schema version = %d, model version = %d", got, model.SchemaVersion)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["additionalProperties"] != false {
		t.Fatalf("schema root contract = %#v", schema)
	}

	encoded, err := json.Marshal(model.Report{})
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	required := stringSlice(t, schema["required"], "required")
	want := make([]string, 0, len(report))
	for key := range report {
		want = append(want, key)
	}
	sort.Strings(required)
	sort.Strings(want)
	if !reflect.DeepEqual(required, want) {
		t.Fatalf("required keys = %#v, report keys = %#v", required, want)
	}

	definitions := object(t, schema["$defs"], "$defs")
	walkSchema(t, schema, func(reference string) {
		const prefix = "#/$defs/"
		if !strings.HasPrefix(reference, prefix) {
			t.Fatalf("non-local schema reference %q", reference)
		}
		if _, exists := definitions[strings.TrimPrefix(reference, prefix)]; !exists {
			t.Fatalf("unresolved schema reference %q", reference)
		}
	})
}

func walkSchema(t *testing.T, value any, visit func(string)) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if reference, ok := typed["$ref"].(string); ok {
			visit(reference)
		}
		for _, nested := range typed {
			walkSchema(t, nested, visit)
		}
	case []any:
		for _, nested := range typed {
			walkSchema(t, nested, visit)
		}
	}
}

func object(t *testing.T, value any, name string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", name, value)
	}
	return object
}

func stringSlice(t *testing.T, value any, name string) []string {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", name, value)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		stringValue, ok := value.(string)
		if !ok {
			t.Fatalf("%s contains %#v, want string", name, value)
		}
		result = append(result, stringValue)
	}
	return result
}
