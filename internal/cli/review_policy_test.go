package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/projectcontract"
	"github.com/Cyberlane/mori/skills"
)

func TestAdvisoryStagedReviewPreservesEvidenceAndCoverage(t *testing.T) {
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	for name, content := range map[string]string{
		"a.go": "package sample\nfunc First(x int) int { return x+1 }\n",
		"b.go": "package sample\nfunc Second(x int) int { return x+2 }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cliTestGit(t, root, "add", ".")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	run := func(policy string, extra ...string) (int, model.Report) {
		t.Helper()
		args := []string{"review", "staged", "check", "--no-config", "--min-tokens", "1", "--format", "json", "--policy", policy}
		args = append(args, extra...)
		args = append(args, ".")
		var out, errOut bytes.Buffer
		code := Run(context.Background(), args, &out, &errOut)
		var result model.Report
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("exit %d: %s / %s", code, &out, &errOut)
		}
		return code, result
	}
	strictCode, strict := run("strict")
	advisoryCode, advisory := run("advisory")
	if strictCode != exitFindings || advisoryCode != exitSuccess || strict.TotalFocusedMatchGroups == 0 || strict.TotalFocusedMatchGroups != advisory.TotalFocusedMatchGroups {
		t.Fatalf("strict/advisory: %d/%d %+v/%+v", strictCode, advisoryCode, strict.Review, advisory.Review)
	}
	if strict.Review.Status != "blocked" || advisory.Review.Status != "passed" || advisory.Review.Analysis != "complete" || advisory.Review.Acknowledged {
		t.Fatalf("outcomes %+v/%+v", strict.Review, advisory.Review)
	}
	if strict.Configuration.Input.IndexDigest != advisory.Configuration.Input.IndexDigest || strict.Configuration.ScanProfileDigest != advisory.Configuration.ScanProfileDigest {
		t.Fatal("policy changed analysis input or baseline profile")
	}
	if err := os.WriteFile(filepath.Join(root, "bad.go"), []byte("package sample\nfunc Broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "bad.go")
	code, result := run("advisory", "--fail-on-parse-diagnostic")
	if code != exitCoverage || result.Review.CoveragePolicyMet || result.Review.Status != "blocked" || result.Review.Analysis != "incomplete" {
		t.Fatalf("advisory coverage: %d %+v", code, result.Review)
	}
}

func TestAdvisoryDoesNotAcceptReceiptOrInvalidPolicy(t *testing.T) {
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	for _, extra := range [][]string{{"--policy", "silent"}, {"--policy", "advisory", "--review-receipt", "receipt.json"}} {
		var out, errOut bytes.Buffer
		args := append([]string{"review", "staged", "check", "--no-config"}, extra...)
		if code := Run(context.Background(), args, &out, &errOut); code != exitUsage {
			t.Fatalf("%v: %d %s", extra, code, &errOut)
		}
	}
}

func TestPriorOfficialReviewContractUpgradesWithoutManualConflict(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &out, &errOut); code != 0 {
		t.Fatalf("%d: %s", code, &errOut)
	}
	path := filepath.Join(root, projectcontract.FileName)
	old, _, err := projectcontract.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	old.ReportSchemaVersion = 20
	old.NormalizationVersion = 12
	old.HookContract = projectcontract.Artifact{Revision: "mori-hook-pre-commit/v1", Digest: "a12b16adf11655b72146d0c34966434a12f5ae7257ab10fc93f735c6c9035cfa"}
	data, err := projectcontract.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := inspectProjectUpgrade(context.Background(), root, "check")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range plan.Components {
		if part.Ownership == "mori-managed" && part.Classification == "conflict/manual" {
			t.Fatalf("known prior contract became conflict: %+v", part)
		}
	}
	out.Reset()
	errOut.Reset()
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &out, &errOut); code != 0 {
		t.Fatalf("%d: %s", code, &errOut)
	}
	current, _, err := projectcontract.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if current.ReportSchemaVersion != model.SchemaVersion || current.HookContract.Revision != "mori-hook-pre-commit/v2" || current.NormalizationVersion != normalize.Version {
		t.Fatalf("not upgraded: %+v", current)
	}
}

// stagedReviewFixture commits background files, stages the focused files and
// returns an advisory staged-review runner rooted in the fixture.
func stagedReviewFixture(t *testing.T, committed, staged map[string]string) func(extra ...string) (int, model.Report) {
	t.Helper()
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	write := func(files map[string]string) {
		for name, content := range files {
			path := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
		cliTestGit(t, root, "add", ".")
	}
	if len(committed) > 0 {
		write(committed)
		cliTestGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	}
	write(staged)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return func(extra ...string) (int, model.Report) {
		t.Helper()
		args := []string{"review", "staged", "check", "--no-config", "--min-tokens", "1", "--format", "json", "--policy", "advisory"}
		args = append(args, extra...)
		args = append(args, ".")
		var out, errOut bytes.Buffer
		code := Run(context.Background(), args, &out, &errOut)
		var result model.Report
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Review == nil {
			t.Fatalf("exit %d: %s / %s", code, &out, &errOut)
		}
		if !sort.StringsAreSorted(result.Review.AnalysisReasons) {
			t.Fatalf("unsorted reasons %v", result.Review.AnalysisReasons)
		}
		return code, result
	}
}

func TestStagedAnalysisTreatsExpectedZeroFragmentFilesAsComplete(t *testing.T) {
	// Generated source is committed background: a staged generated file is a
	// focused-coverage gap under the staged contract, which is unchanged here.
	run := stagedReviewFixture(t, map[string]string{
		"generated.go": "// Code generated by fixture. DO NOT EDIT.\n\npackage sample\nfunc Generated(x int) int { return x+3 }\n",
	}, map[string]string{
		"a.go":             "package sample\nfunc First(x int) int { return x+1 }\n",
		"b.go":             "package sample\nfunc Second(x int) int { return x+2 }\n",
		"constants.go":     "package sample\nconst Limit = 10\n",
		"types.ts":         "export type Photo = { id: string; width: number };\n",
		"migrations/1.sql": "CREATE TABLE photos (id TEXT PRIMARY KEY);\nCREATE INDEX photos_id ON photos (id);\n",
		"queries/1.sql":    "SELECT id FROM photos WHERE id = ?;\n",
	})
	code, result := run("--exclude-generated")
	if code != exitSuccess || result.Review.Analysis != "complete" || len(result.Review.AnalysisReasons) != 0 {
		t.Fatalf("code %d review %+v coverage %+v warnings %+v", code, result.Review, result.Coverage, result.Warnings)
	}
	if result.Coverage.ZeroFragmentFiles == 0 || result.Coverage.GeneratedExcluded == 0 {
		t.Fatalf("fixture no longer exercises benign gaps: %+v", result.Coverage)
	}
}

func TestStagedAnalysisTreatsDefinitionOnlySQLAsComplete(t *testing.T) {
	run := stagedReviewFixture(t, nil, map[string]string{
		"migrations/1.sql": "CREATE TABLE photos (id TEXT PRIMARY KEY);\nCREATE INDEX photos_id ON photos (id);\n",
		"queries/1.sql":    "SELECT id FROM photos WHERE id = ?;\n",
	})
	code, result := run("--comparison-domain", "sql-query")
	if code != exitSuccess || result.Review.Analysis != "complete" || result.Coverage.ZeroFragmentFiles != 1 {
		t.Fatalf("definition-only SQL: code %d review %+v coverage %+v warnings %+v", code, result.Review, result.Coverage, result.Warnings)
	}
}

func TestStagedAnalysisReportsEvidenceGapReasons(t *testing.T) {
	run := stagedReviewFixture(t, nil, map[string]string{
		"a.go":         "package sample\nfunc First(x int) int { return x+1 }\n",
		"b.go":         "package sample\nfunc Second(x int) int { return x+2 }\n",
		"c.go":         "package sample\nfunc Third(x int) int { if x > 0 { return x }; return -x }\n",
		"d.go":         "package sample\nfunc Fourth(y int) int { if y > 0 { return y }; return -y }\n",
		"constants.go": "package sample\nconst Limit = 10\n",
		"large.go":     "package sample\n\n// Padding keeps this file above the fixture byte limit.\nfunc Large(x int) int { if x > 0 { return x }; return -x }\n",
	})
	code, result := run("--max-zero-fragment-files", "0")
	if code != exitCoverage || result.Review.Analysis != "incomplete" || !reflect.DeepEqual(result.Review.AnalysisReasons, []string{"coverage_policy_unmet"}) {
		t.Fatalf("zero-fragment policy: %d %+v", code, result.Review)
	}
	_, result = run("--max-groups", "1")
	if !result.Truncated || !reflect.DeepEqual(result.Review.AnalysisReasons, []string{"truncated"}) {
		t.Fatalf("truncated: %+v", result.Review)
	}
	_, result = run("--max-file-bytes", "100")
	// A staged oversized file is also a focused-coverage gap.
	if !reflect.DeepEqual(result.Review.AnalysisReasons, []string{"coverage_policy_unmet", "unanalyzed_files", "warnings"}) {
		t.Fatalf("oversized: %+v %+v", result.Review, result.Coverage)
	}
	if err := os.WriteFile("bad.go", []byte("package sample\nfunc Broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, ".", "add", "bad.go")
	_, result = run()
	if result.Review.Analysis != "incomplete" || !slices.Contains(result.Review.AnalysisReasons, "parse_diagnostics") {
		t.Fatalf("parse gap: %+v", result.Review)
	}
}

// A v0.34.1 project records report schema 22 and its own official skill
// package. Advancing the report contract must remain a normal managed migration.
func TestReportSchema22ProjectUpgradesToCurrentContract(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &out, &errOut); code != 0 {
		t.Fatalf("%d: %s", code, &errOut)
	}
	path := filepath.Join(root, projectcontract.FileName)
	old, _, err := projectcontract.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	skillRoot := filepath.Join(root, ".agents", "skills", skills.ReviewSimilarityName)
	prior := []byte("Set `MORI_REPORT` to an owner-private temporary or Git-metadata path.\n")
	if err := os.WriteFile(filepath.Join(skillRoot, "references", "changed-code.md"), prior, 0600); err != nil {
		t.Fatal(err)
	}
	old.MoriVersion, old.EmbeddedSkill.Revision = "v0.34.1", "v0.34.1"
	old.ReportSchemaVersion = 22
	if old.EmbeddedSkill.Digest, err = installedSkillDigest(skillRoot); err != nil {
		t.Fatal(err)
	}
	data, err := projectcontract.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mori-version"), []byte("v0.34.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := inspectProjectUpgrade(context.Background(), root, "check")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range plan.Components {
		if part.Ownership == "mori-managed" && part.Classification == "conflict/manual" {
			t.Fatalf("prior report-22 contract became conflict: %+v", part)
		}
	}
	for _, args := range [][]string{{"project", "upgrade", "--apply", root}, {"project", "upgrade", "--check", root}} {
		out.Reset()
		errOut.Reset()
		if code := Run(context.Background(), args, &out, &errOut); code != 0 {
			t.Fatalf("%v: %d %s", args, code, &errOut)
		}
	}
	current, _, err := projectcontract.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if current.ReportSchemaVersion != model.SchemaVersion ||
		current.ReviewReceiptSchema != old.ReviewReceiptSchema || current.BaselineSchema != old.BaselineSchema ||
		current.NormalizationVersion != old.NormalizationVersion {
		t.Fatalf("unexpected contract migration: %+v -> %+v", old, current)
	}
	installed, err := os.ReadFile(filepath.Join(skillRoot, "references", "changed-code.md"))
	if err != nil || !bytes.Contains(installed, []byte("--output auto")) {
		t.Fatal("upgrade did not install managed-output guidance")
	}
}
