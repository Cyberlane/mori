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

// TestAcceptanceReviewJourney crosses discovery, parsing, ranking, identities,
// baseline persistence and support export using only the public CLI dispatcher.
func TestAcceptanceReviewJourney(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"packages/a/store.ts": `export function select(items: number[]) { let total = 0; for (const item of items) { if (item > 0) { total += item * 2; } } return total; }
export function dateKey(value: string) { return value.trim().toLowerCase(); }`,
		"packages/b/store.ts": `export function gather(values: number[]) { let sum = 0; for (const value of values) { if (value > 0) { sum += value * 2; } } return sum; }
export function normalizeDate(input: string) { return input.trim().toLowerCase(); }`,
		"packages/a/store.test.ts": `function testHelper(values: number[]) { let sum = 0; for (const value of values) { if (value > 0) { sum += value * 2; } } return sum; }`,
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(want int, args ...string) []byte {
		t.Helper()
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, &out, &stderr); code != want {
			t.Fatalf("%v: exit %d want %d: %s", args, code, want, &stderr)
		}
		return out.Bytes()
	}
	scan := func(extra ...string) model.Report {
		t.Helper()
		args := append([]string{"scan", "--no-config", "--format", "json", "--threshold", "0.85"}, extra...)
		var report model.Report
		if err := json.Unmarshal(run(exitSuccess, append(args, root)...), &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	high := scan("--min-tokens", "30")
	if len(high.Groups) == 0 {
		t.Fatal("high floor lost large duplicate")
	}
	low := scan("--min-tokens", "1", "--fragment-selection", "production")
	if len(low.Groups) <= len(high.Groups) {
		t.Fatalf("lower floor did not recover small finding: high=%d low=%d", len(high.Groups), len(low.Groups))
	}
	excluded := 0
	for _, file := range low.FileCoverage {
		excluded += file.ExcludedTestFragments
	}
	if excluded != 1 {
		t.Fatalf("production selection excluded %d test fragments", excluded)
	}
	for _, old := range high.Groups {
		found := false
		for _, current := range low.Groups {
			if old.ID == current.ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("tuning changed stable identity %s", old.ID)
		}
	}
	common := []string{"--no-config", "--threshold", "0.85", "--min-tokens", "1", "--fragment-selection", "production"}
	id := low.Groups[0].ID
	run(exitSuccess, append(append([]string{"explain", id}, common...), root)...)
	baseline := filepath.Join(root, "accepted.json")
	run(exitSuccess, append(append([]string{"baseline", "add", "--baseline", baseline, "--identity", id, "--classification", "intentional"}, common...), root)...)
	accepted := scan("--min-tokens", "1", "--fragment-selection", "production", "--baseline", baseline)
	if accepted.SuppressedMatchGroups != 1 || accepted.TotalMatchGroups != low.TotalMatchGroups-1 {
		t.Fatalf("selective suppression: %+v", accepted)
	}
	// A later invalid identity must leave the entire existing baseline untouched.
	before, err := os.ReadFile(baseline)
	if err != nil {
		t.Fatal(err)
	}
	run(exitError, append(append([]string{"baseline", "add", "--baseline", baseline, "--identity", low.Groups[1].ID, "--identity", "missing-identity"}, common...), root)...)
	after, err := os.ReadFile(baseline)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed batch changed baseline")
	}
	// Repeated identities are idempotent, and selected groups are accepted together.
	run(exitSuccess, append(append([]string{"baseline", "add", "--baseline", baseline, "--identity", id, "--identity", low.Groups[1].ID, "--identity", id}, common...), root)...)
	all := scan("--min-tokens", "1", "--fragment-selection", "production", "--baseline", baseline)
	if all.SuppressedMatchGroups != 2 {
		t.Fatalf("batch suppression = %d", all.SuppressedMatchGroups)
	}
	// Profile failure must happen before candidate enumeration can hit its limit.
	var mismatchOut, mismatchErr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--no-config", "--threshold", "0.86", "--min-tokens", "1", "--max-pairs", "1", "--baseline", baseline, root}, &mismatchOut, &mismatchErr)
	if code != exitError || !strings.Contains(mismatchErr.String(), "profile") || strings.Contains(mismatchErr.String(), "candidate pair limit") {
		t.Fatalf("late compatibility rejection: %d %s", code, &mismatchErr)
	}
	truncated := scan("--min-tokens", "1", "--max-groups", "1")
	if !truncated.Truncated || len(truncated.Groups) != 1 {
		t.Fatal("bounded report did not disclose truncation")
	}
	run(exitError, "scan", "--no-config", "--min-tokens", "1", "--max-pairs", "1", root)
	session := filepath.Join(root, "session.json")
	scan("--min-tokens", "1", "--fragment-selection", "production", "--diagnostics", session)
	bundle := filepath.Join(root, "support.zip")
	preview := run(exitSuccess, "support", "bundle", "--session", session, "--output", bundle, "--finding", "1:useful")
	inspected := run(exitSuccess, "support", "inspect", bundle)
	if !bytes.Equal(preview, inspected) {
		t.Fatal("inspection differs from exported evidence")
	}
	for _, sensitive := range []string{root, "store.ts", "dateKey", "number[]"} {
		if strings.Contains(string(inspected), sensitive) {
			t.Fatalf("support leaked %q", sensitive)
		}
	}
	// Parser damage remains visible and blocks durable acceptance by default.
	broken := filepath.Join(root, "broken.ts")
	if err := os.WriteFile(broken, []byte("function broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	damaged := scan("--min-tokens", "1", "--fragment-selection", "production")
	if len(damaged.Warnings) == 0 {
		t.Fatal("parser damage was silent")
	}
	before, err = os.ReadFile(baseline)
	if err != nil {
		t.Fatal(err)
	}
	run(exitCoverage, append(append([]string{"baseline", "add", "--baseline", baseline, "--identity", id}, common...), root)...)
	after, err = os.ReadFile(baseline)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("warning-bearing scan changed acceptance")
	}

}

func TestAcceptanceBatchBaselineSkillUpgradeLifecycle(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("upgrade: %d %s", code, &stderr)
	}
	installed := filepath.Join(root, ".agents", "skills", "mori-review-similarity", "references", "baselines-receipts.md")
	content, err := os.ReadFile(installed)
	if err != nil || !bytes.Contains(content, []byte("Repeat `--identity`")) || !bytes.Contains(content, []byte("one atomic baseline write")) {
		t.Fatalf("missing batch review contract: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"project", "upgrade", "--check", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("upgraded contract: %d %s", code, &stderr)
	}
}
