package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestPlanAcceptanceWorkloadAndPrivacy(t *testing.T) {
	root := t.TempDir()
	for _, pkg := range []string{"alpha", "beta"} {
		dir := filepath.Join(root, "packages", pkg)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		content := "function privateName(x: number) { return x * 2; }\nfunction otherName(y: number) { return y * 3; }"
		if err := os.WriteFile(filepath.Join(dir, "private.ts"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(want int, args ...string) []byte {
		t.Helper()
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, &out, &stderr); code != want {
			t.Fatalf("%v: %d want %d %s", args, code, want, &stderr)
		}
		return out.Bytes()
	}
	raw := run(exitSuccess, "plan", "--no-config", "--format", "json", "--min-tokens", "1", "--max-pairs", "1", root)
	var plan scanPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	var report model.Report
	if err := json.Unmarshal(run(exitSuccess, "scan", "--no-config", "--format", "json", "--min-tokens", "1", "--max-pairs", "0", root), &report); err != nil {
		t.Fatal(err)
	}
	if plan.CandidatePairs != report.CandidatePairs || !plan.ExceedsPairLimit || plan.AnalysisPerformed || plan.Artifact != "mori-scan-plan" || len(plan.Packages) != 2 {
		t.Fatalf("incorrect planning evidence: %+v", plan)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"groups", "total_match_groups", "review", "suppressed_match_groups"} {
		if _, found := fields[forbidden]; found {
			t.Fatalf("plan masquerades as scan: %s", forbidden)
		}
	}
	var narrow scanPlan
	if err := json.Unmarshal(run(exitSuccess, "plan", "--no-config", "--format", "json", "--min-tokens", "1", filepath.Join(root, "packages", "alpha")), &narrow); err != nil {
		t.Fatal(err)
	}
	if narrow.CandidatePairs >= plan.CandidatePairs || narrow.Files != 1 {
		t.Fatalf("scope did not reduce work: %+v", narrow)
	}
	redacted := run(exitSuccess, "plan", "--no-config", "--format", "json", "--min-tokens", "1", "--redact-paths", root)
	for _, value := range []string{root, "alpha", "beta", "private.ts", "privateName"} {
		if bytes.Contains(redacted, []byte(value)) {
			t.Fatalf("redaction leaked %q", value)
		}
	}
	text := run(exitSuccess, "plan", "--no-config", "--min-tokens", "1", root)
	if !bytes.Contains(text, []byte("No similarities scored")) || !bytes.Contains(text, []byte("omit cross-package matches")) {
		t.Fatalf("missing plan boundaries: %s", text)
	}
	for _, args := range [][]string{
		{"--diagnostics", filepath.Join(root, "session.json")}, {"--output", filepath.Join(root, "report.json")}, {"--staged"}, {"--format", "agent"}, {"--unknown"}, {"--focus-path", root},
	} {
		run(exitUsage, append(append([]string{"plan", "--no-config"}, args...), root)...)
	}
	// Exactly the fixture files remain after all successful and rejected commands.
	count := 0
	if err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("planning wrote artifacts: %d files", count)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.ts"), []byte("function broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	damaged := run(exitCoverage, "plan", "--no-config", "--fail-on-warning", root)
	if !strings.Contains(string(damaged), "parse diagnostic") {
		t.Fatalf("missing parser coverage: %s", damaged)
	}
}

func TestPlanHonorsProjectContract(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("upgrade %d %s", code, &stderr)
	}
	skill := filepath.Join(root, ".agents", "skills", "mori-review-similarity", "SKILL.md")
	if err := os.WriteFile(skill, []byte("locally modified"), 0600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"plan", root}, &stdout, &stderr); code != exitUpgrade {
		t.Fatalf("drift not enforced: %d %s", code, &stderr)
	}
}

func TestPlanSchemaMatchesArtifact(t *testing.T) {
	raw, err := os.ReadFile("../../schemas/mori-scan-plan-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(scanPlan{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != len(schema.Required) || len(fields) != len(schema.Properties) {
		t.Fatal("schema fields differ from plan artifact")
	}
	for _, name := range schema.Required {
		if _, ok := fields[name]; !ok {
			t.Fatalf("schema requires missing field %s", name)
		}
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("schema omits property %s", name)
		}
	}
}
