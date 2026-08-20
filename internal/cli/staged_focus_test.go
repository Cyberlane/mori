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

func TestCanonicalStagedReviewScoresOnlyHunkPairsAndKeepsUnsupportedPathEvidence(t *testing.T) {
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	cliTestGit(t, root, "config", "user.name", "Mori Test")
	cliTestGit(t, root, "config", "user.email", "mori@example.invalid")
	base := `package sample

func Changed(value int) int {
	if value > 0 {
		return value + 1
	}
	return 0
}

func Untouched(value int) int {
	if value > 0 {
		return value + 1
	}
	return 0
}

func Existing(value int) int {
	if value > 0 {
		return value + 1
	}
	return 0
}
`
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "sample.go")
	cliTestGit(t, root, "commit", "-m", "base")
	changed := strings.Replace(base, "return value + 1", "return value + 2", 1)
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Package.resolved"), []byte("unsupported staged asset\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "sample.go", "Package.resolved")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"review", "staged", "check", "--no-config", "--format", "json", "--min-tokens", "1", ".",
	}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("staged check exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !result.Configuration.FocusedOnly || result.CandidatePairs != 2 || result.TotalLocationPairs != 2 || result.TotalFocusedMatchGroups != 1 {
		t.Fatalf("focused-only totals = candidates %d locations %d focused %d config %+v", result.CandidatePairs, result.TotalLocationPairs, result.TotalFocusedMatchGroups, result.Configuration)
	}
	if result.Configuration.Focus == nil {
		t.Fatal("missing focus evidence")
	}
	var sourceEvidence, assetEvidence *model.FocusPathEvidence
	for index := range result.Configuration.Focus.PathEvidence {
		evidence := &result.Configuration.Focus.PathEvidence[index]
		switch evidence.Path {
		case "sample.go":
			sourceEvidence = evidence
		case "Package.resolved":
			assetEvidence = evidence
		}
	}
	if sourceEvidence == nil || sourceEvidence.Status != "analyzed" || len(sourceEvidence.ChangedLines) != 1 ||
		sourceEvidence.ChangedLines[0].StartLine != 5 || sourceEvidence.ChangedLines[0].EndLine != 5 {
		t.Fatalf("source focus evidence = %#v", sourceEvidence)
	}
	if assetEvidence == nil || assetEvidence.Status != "unsupported" {
		t.Fatalf("asset focus evidence = %#v", assetEvidence)
	}
	for _, warning := range result.Warnings {
		if warning.Path == "Package.resolved" || strings.Contains(warning.Message, "unsupported source extension") {
			t.Fatalf("unsupported staged asset became a warning: %#v", warning)
		}
	}
}

func TestCanonicalStagedReviewKeepsExistingBaselineProfileCompatible(t *testing.T) {
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	cliTestGit(t, root, "config", "user.name", "Mori Test")
	cliTestGit(t, root, "config", "user.email", "mori@example.invalid")
	for name, function := range map[string]string{
		"left.go":  "func Left(value int) int { return value + 1 }\n",
		"right.go": "func Right(value int) int { return value + 1 }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package sample\n"+function), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cliTestGit(t, root, "add", "left.go", "right.go")
	cliTestGit(t, root, "commit", "-m", "base")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	baselinePath := filepath.Join(root, ".mori-baseline.json")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"baseline", "update", "--accept-all", "--no-config", "--baseline", baselinePath,
		"--threshold", "1", "--min-tokens", "1", "--same-language-only", ".",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("baseline update exit = %d, stderr = %q", code, stderr.String())
	}
	config := `{"threshold":1,"min_tokens":1,"same_language_only":true,"baseline":".mori-baseline.json"}`
	if err := os.WriteFile(filepath.Join(root, ".mori.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", ".mori.json", ".mori-baseline.json")
	cliTestGit(t, root, "commit", "-m", "baseline")
	if err := os.WriteFile(filepath.Join(root, "left.go"), []byte("package sample\nfunc Left(value int) int { return value + 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "left.go")

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"review", "staged", "check", "--format", "json", "."}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("canonical check exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Configuration.FocusedOnly || result.Configuration.BaselineStatus != "compatible" ||
		result.TotalMatchGroups != 0 || result.SuppressedMatchGroups != 1 {
		t.Fatalf("baseline-compatible focused report = %#v", result)
	}
}
