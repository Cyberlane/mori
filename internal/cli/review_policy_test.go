package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/projectcontract"
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
