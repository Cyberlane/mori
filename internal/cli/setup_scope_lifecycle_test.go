package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/projectcontract"
)

func TestGeneratedLibraryScopeSurvivesProjectUpgrade(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(name string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("export function double(x: number) { return x * 2; }\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("packages/core/src/main.ts")
	write("packages/core/src/current.test.ts")
	write("demo/main.ts")
	var out, stderr bytes.Buffer
	run := func(args ...string) {
		t.Helper()
		out.Reset()
		stderr.Reset()
		if code := Run(context.Background(), args, &out, &stderr); code != exitSuccess {
			t.Fatalf("%v: %d %s %s", args, code, out.String(), stderr.String())
		}
	}
	if code := RunWithInput(context.Background(), []string{"setup", "--answers", "-", "--apply", root}, strings.NewReader(`{"review_intent":"library"}`), &out, &stderr); code != exitSuccess {
		t.Fatalf("setup %d: %s", code, stderr.String())
	}
	configPath := filepath.Join(root, config.FileName)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	scope := policy.Scopes["library"]
	if !reflect.DeepEqual(scope.Roots, []string{"packages/core/src"}) || !reflect.DeepEqual(scope.Excludes, []string{"**/*.test.*"}) {
		t.Fatalf("scope %+v", scope)
	}
	check := func() {
		t.Helper()
		run("project", "upgrade", "--check", "--format", "json", root)
		var plan projectUpgradePlan
		if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
			t.Fatal(err)
		}
		if plan.Drift {
			t.Fatalf("unexpected contract drift: %+v", plan)
		}
		after, err := os.ReadFile(configPath)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("upgrade changed generated scope policy: %v", err)
		}
	}
	run("project", "upgrade", "--apply", root)
	check()
	contractPath := filepath.Join(root, projectcontract.FileName)
	prior, _, err := projectcontract.Load(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	prior.ConfigSchemaVersion = 1
	prior.ReportSchemaVersion = 21
	prior.NormalizationVersion = 13
	raw, err := projectcontract.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	run("project", "upgrade", "--apply", root)
	check()
	// A newly added matching test stays excluded after migration, while new
	// production source inside the observed library root is still discovered.
	write("packages/core/src/nested/future.test.ts")
	write("packages/core/src/nested/future.ts")
	run("scan", "--config", configPath, "--scope", "library", "--min-tokens", "1", "--format", "json")
	var report model.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Files != 2 || len(report.FileCoverage) != 2 {
		t.Fatalf("expected only production source: %+v", report.FileCoverage)
	}
	seenFuture := false
	for _, file := range report.FileCoverage {
		if strings.Contains(file.Path, ".test.") || strings.Contains(file.Path, "demo") {
			t.Fatalf("scope expanded unexpectedly: %s", file.Path)
		}
		seenFuture = seenFuture || strings.HasSuffix(file.Path, "future.ts")
	}
	if !seenFuture {
		t.Fatal("new production source not discovered")
	}
}
