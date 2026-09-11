package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestUsabilityArtifactsMatchCurrentContracts(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	doc, err := os.ReadFile(filepath.Join(root, "docs/machine-integration.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), fmt.Sprintf("schemas/mori-report-v%d.schema.json", model.SchemaVersion)) {
		t.Fatal("machine guide does not link current schema")
	}
	raw, err := os.ReadFile(filepath.Join(root, "schemas/mori-scan-failure-v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(scanFailure{})
	if err != nil {
		t.Fatal(err)
	}
	var artifact map[string]any
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		t.Fatal(err)
	}
	var actual, required []string
	for key := range artifact {
		actual = append(actual, key)
	}
	for _, key := range schema["required"].([]any) {
		required = append(required, key.(string))
	}
	sort.Strings(actual)
	sort.Strings(required)
	if !reflect.DeepEqual(actual, required) {
		t.Fatalf("failure artifact/schema mismatch %v vs %v", actual, required)
	}
	props := schema["properties"].(map[string]any)
	if props["complete"].(map[string]any)["const"] != false || props["artifact"].(map[string]any)["const"] != "mori-scan-failure" {
		t.Fatal("failure schema permits complete reports")
	}
}
