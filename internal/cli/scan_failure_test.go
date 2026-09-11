package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCandidateLimitRetainsExplicitFailureEvidence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		content := fmt.Sprintf("package fixture\nfunc Value%d(x int) int { return x+1 }\n", i)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("v%d.go", i)), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(t.TempDir(), "evidence.json")
	for _, format := range []string{"json", "agent"} {
		t.Run(format, func(t *testing.T) {
			var out, stderr bytes.Buffer
			args := []string{"scan", "--no-config", "--min-tokens", "1", "--max-pairs", "1", "--format", format}
			if format == "agent" {
				args = append(args, "--output", output)
			}
			args = append(args, root)
			code := Run(context.Background(), args, &out, &stderr)
			if code != exitError {
				t.Fatalf("%d %s", code, stderr.String())
			}
			data := out.Bytes()
			if format == "agent" {
				var err error
				data, err = os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
			}
			var failure scanFailure
			if err := json.Unmarshal(data, &failure); err != nil {
				t.Fatalf("%v %s", err, data)
			}
			if failure.Artifact != "mori-scan-failure" || failure.Complete || failure.Compared != 1 || failure.Coverage.AnalyzedFiles != 3 || failure.Configuration.MinTokens != 1 || failure.Tool.NormalizationVersion == 0 {
				t.Fatalf("failure %+v", failure)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"groups", "review", "schema_version"} {
				if _, ok := raw[key]; ok {
					t.Fatalf("failure masquerades as report via %s", key)
				}
			}
			if strings.Contains(out.String(), "complete JSON evidence:") && !strings.Contains(out.String(), "incomplete failure evidence:") {
				t.Fatal("claimed success")
			}
		})
	}
	// Narrow retry succeeds with a complete report; output bounds do not bypass compute bounds.
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"scan", "--no-config", "--min-tokens", "1", "--max-pairs", "1", "--format", "json", filepath.Join(root, "v0.go")}, &out, &stderr); code != 0 {
		t.Fatalf("retry %d %s", code, stderr.String())
	}
	if strings.Contains(out.String(), "failure_schema_version") {
		t.Fatal("successful retry is a failure")
	}
}

func TestCandidateFailureOutputErrorRemainsFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package fixture\nfunc A(x int) int{return x+1}\nfunc B(x int) int{return x+1}\nfunc C(x int) int{return x+1}"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--no-config", "--min-tokens", "1", "--max-pairs", "1", "--format", "agent", "--output", filepath.Join(root, "missing", "report.json"), root}, &out, &stderr)
	if code != exitError || strings.Contains(out.String(), "evidence:") || !strings.Contains(stderr.String(), "write incomplete failure evidence") {
		t.Fatalf("%d %s %s", code, out.String(), stderr.String())
	}
}
