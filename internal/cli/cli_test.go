package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRunLanguages(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"languages"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	for _, expected := range []string{"Go", "JavaScript / JSX", "Python", "Rust", "SQL queries", "TypeScript / TSX", "sql-query"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("languages output missing %q", expected)
		}
	}
}

func TestRunScanSQLQueries(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--threshold", "0.70", "--min-tokens", "12",
		"../../examples/sql-queries",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.Files != 4 || result.Fragments != 4 || result.TotalMatchGroups != 1 ||
		len(result.Groups) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("SQL report summary = %+v", result)
	}
	profile := result.Groups[0].Profiles[0]
	if profile.OccurrenceCount != 2 || len(profile.Occurrences) != 2 {
		t.Fatalf("SQL profile = %+v", profile)
	}
	for _, occurrence := range profile.Occurrences {
		location := occurrence.Location
		if location.Language != "sql" || location.LanguageFamily != "sql" ||
			location.ComparisonDomain != "sql-query" || location.FragmentKind != "query" {
			t.Fatalf("SQL location = %+v", location)
		}
	}
}

func TestRunScanJSONAndFailOnMatch(t *testing.T) {
	t.Parallel()

	args := []string{
		"scan",
		"--threshold", "0.70",
		"--cross-language-only",
		"--format", "json",
		"--fail-on-match",
		"../../examples/email-validation",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitFindings, stderr.String())
	}

	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if len(result.Groups) == 0 {
		t.Fatal("JSON report contains no groups")
	}
	if result.Warnings == nil {
		t.Fatal("warnings must encode as an empty array, not null")
	}
	if result.SchemaVersion != model.SchemaVersion || result.Tool.Name != "mori" || result.Tool.Revision == "" ||
		result.Tool.NormalizationVersion != normalize.Version {
		t.Fatalf("report provenance = schema %d, tool %#v", result.SchemaVersion, result.Tool)
	}
}

func TestRunScanFocusPathAndFocusedPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	left := filepath.Join(root, "left.go")
	right := filepath.Join(root, "right.go")
	content := "package sample\nfunc Same(value string) string { if value == \"\" { return \"\" }; return value }\n"
	for _, file := range []string{left, right} {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--threshold", "1", "--min-tokens", "1", "--format", "json",
		"--focus-path", left, "--fail-on-focused-match", root,
	}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.Configuration.Focus == nil || result.Configuration.Focus.Mode != "explicit" ||
		result.Configuration.Focus.DiscoveredFocusFiles != 1 || result.TotalFocusedMatchGroups != 1 ||
		len(result.Groups) != 1 || !result.Groups[0].Focused || result.Groups[0].FocusedCount != 1 {
		t.Fatalf("focused report = %+v", result)
	}
}

func TestRunScanChangedSinceIncludesWorkingTree(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Mori Test")
	runTestGit(t, root, "config", "user.email", "mori@example.invalid")
	left := filepath.Join(root, "left.go")
	right := filepath.Join(root, "right.go")
	content := "package sample\nfunc Same(value string) string { if value == \"\" { return \"\" }; return value }\n"
	for _, file := range []string{left, right} {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	runTestGit(t, root, "add", ".")
	runTestGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(left, []byte(content+"// changed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile change: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--threshold", "1", "--min-tokens", "1", "--format", "json",
		"--changed-since", base, "--fail-on-focused-match", root,
	}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	focus := result.Configuration.Focus
	if focus == nil || focus.Mode != "git" || focus.RequestedBase != base ||
		focus.BaseCommit != base || focus.MergeBase != base || len(focus.HeadCommit) != 40 ||
		!focus.WorkingTreeIncluded || !focus.UntrackedIncluded ||
		!reflect.DeepEqual(focus.ChangedPaths, []string{"left.go"}) ||
		focus.DiscoveredFocusFiles != 1 || result.TotalFocusedMatchGroups != 1 {
		t.Fatalf("Git focus report = %+v", result)
	}
}

func TestFocusPolicyValidation(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"scan", "--fail-on-match", "--fail-on-focused-match", "--focus-path", "file.go", "."},
		{"scan", "--fail-on-focused-match", "."},
		{"baseline", "update", "--baseline", "accepted.json", "--focus-path", "file.go", "."},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := Run(context.Background(), args, &stdout, &stderr); code != exitUsage {
			t.Errorf("Run(%v) exit = %d, stderr = %q", args, code, stderr.String())
		}
	}
}

func runTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestBaselineUpdateAndScanSuppression(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join(t.TempDir(), "mori-baseline.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"baseline", "update",
		"--baseline", baselinePath,
		"--threshold", "0.70",
		"--cross-language-only",
		"../../examples/email-validation",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("baseline update exit = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline was not written: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"scan",
		"--baseline", baselinePath,
		"--threshold", "0.70",
		"--cross-language-only",
		"--fail-on-match",
		"--format", "json",
		"../../examples/email-validation",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("suppressed scan exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode suppressed JSON: %v\n%s", err, stdout.String())
	}
	if result.TotalMatchGroups != 0 || result.SuppressedLocationPairs == 0 || result.Truncated {
		t.Fatalf("suppressed report = %+v, want no findings and visible suppression", result)
	}
}

func TestBaselinePruneCheckReportsStaleEntries(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join(t.TempDir(), "mori-baseline.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	updateArgs := []string{
		"baseline", "update",
		"--baseline", baselinePath,
		"--threshold", "0.70",
		"--cross-language-only",
		"../../examples/email-validation",
	}
	if code := Run(context.Background(), updateArgs, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("baseline update exit = %d, stderr = %q", code, stderr.String())
	}
	before, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile before prune check: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code := Run(context.Background(), []string{
		"baseline", "prune", "--baseline", baselinePath, "--check",
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation/validator.js",
	}, &stdout, &stderr)
	if code != exitFindings || !strings.Contains(stdout.String(), "stale") {
		t.Fatalf("prune check exit/output = %d/%q, stderr = %q", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile after prune check: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("prune --check modified the baseline")
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"baseline", "prune", "--baseline", baselinePath,
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation/validator.js",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("prune exit = %d, stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"baseline", "prune", "--baseline", baselinePath, "--check",
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation/validator.js",
	}, &stdout, &stderr)
	if code != exitSuccess || !strings.Contains(stdout.String(), "baseline is current") {
		t.Fatalf("post-prune check exit/output = %d/%q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestScanRejectsMissingBaseline(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--baseline", filepath.Join(t.TempDir(), "missing.json"), ".",
	}, &stdout, &stderr)
	if code != exitError || !strings.Contains(stderr.String(), "load baseline") {
		t.Fatalf("missing baseline exit/stderr = %d/%q", code, stderr.String())
	}
}

func TestRunRejectsInvalidThreshold(t *testing.T) {
	t.Parallel()

	for _, threshold := range []string{"70", "NaN", "+Inf"} {
		threshold := threshold
		t.Run(threshold, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"scan", "--threshold", threshold, "."},
				&stdout,
				&stderr,
			)
			if code != exitUsage {
				t.Fatalf("exit = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr.String(), "--threshold") {
				t.Fatalf("stderr = %q, want threshold explanation", stderr.String())
			}
		})
	}
}

func TestRunRejectsIncompatibleLanguageDomains(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--language-pair", "sql,go", ".",
	}, &stdout, &stderr)
	if code != exitUsage || !strings.Contains(stderr.String(), "incompatible comparison domains") {
		t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
	}
}

func TestConfigLoadsBeforeCLIOverridesAndLanguagePairsExpand(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, ".mori.json")
	if err := os.WriteFile(configPath, []byte(`{
  "threshold": 0.5,
  "max_groups": 250,
  "language_pairs": ["go,typescript"],
  "exclude": ["generated/**"],
  "respect_ignore": true,
  "baseline": "accepted.json"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stderr bytes.Buffer
	options, _, code, ok := parseScanOptions("scan", []string{
		"--config", configPath,
		"--threshold", "0.9",
		"--no-ignore",
	}, &stderr, false)
	if !ok || code != exitSuccess {
		t.Fatalf("parse = %t/%d, stderr = %q", ok, code, stderr.String())
	}
	if options.threshold != 0.9 || options.maxGroups != 250 || options.respectIgnore ||
		len(options.languagePairs) != 1 || len(options.excludes) != 1 ||
		options.baselinePath != filepath.Join(root, "accepted.json") {
		t.Fatalf("options = %#v", options)
	}
	pairs, err := expandLanguagePairs(options.languagePairs)
	if err != nil {
		t.Fatalf("expandLanguagePairs: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("expanded pairs = %#v, want Go paired with TS and TSX", pairs)
	}
}

func TestRunRejectsUnknownConfigField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".mori.json")
	if err := os.WriteFile(path, []byte(`{"future":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--config", path, "."}, &stdout, &stderr)
	if code != exitError || !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
	}
}

func TestRunAppliesConfigAndIgnoreFilesEndToEnd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		".mori.json":                            `{"threshold":1,"min_tokens":1,"max_groups":10}`,
		".gitignore":                            "generated/\n",
		"left.go":                               "package sample\nfunc Left(value string) string { if value == \"\" { return \"\" }; return value }\n",
		"right.go":                              "package sample\nfunc Right(input string) string { if input == \"\" { return \"\" }; return input }\n",
		filepath.Join("generated", "broken.ts"): "export function broken(: {\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--config", filepath.Join(root, ".mori.json"), "--format", "json", root,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if result.Files != 2 || result.TotalMatchGroups != 1 || len(result.Warnings) != 0 ||
		len(result.Configuration.IgnoreFiles) != 1 || result.Configuration.ConfigPath == "" {
		t.Fatalf("report = %+v", result)
	}
}

func TestRunSkillInstallTarget(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent},
		&stdout,
		&stderr,
	)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Installed mori-review-similarity") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(parent, "mori-review-similarity", "SKILL.md")); err != nil {
		t.Fatalf("Stat(installed SKILL.md): %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent},
		&stdout,
		&stderr,
	)
	if code != exitSuccess || !strings.Contains(stdout.String(), "is current") {
		t.Fatalf("current exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}

	skillPath := filepath.Join(parent, "mori-review-similarity", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("custom\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(custom): %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent},
		&stdout,
		&stderr,
	)
	if code != exitError || !strings.Contains(stderr.String(), "--replace") {
		t.Fatalf("different exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent, "--replace"},
		&stdout,
		&stderr,
	)
	if code != exitSuccess || !strings.Contains(stdout.String(), "Previous copy:") {
		t.Fatalf("replace exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestRunSkillInstallProject(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"skill", "install", "--project", project},
		&stdout,
		&stderr,
	)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	installed := filepath.Join(
		project,
		".agents",
		"skills",
		"mori-review-similarity",
		"SKILL.md",
	)
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("Stat(project SKILL.md): %v", err)
	}
}

func TestRunSkillInstallGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"skill", "install", "--global"},
		&stdout,
		&stderr,
	)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	installed := filepath.Join(
		home,
		".agents",
		"skills",
		"mori-review-similarity",
		"SKILL.md",
	)
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("Stat(global SKILL.md): %v", err)
	}
}

func TestRunSkillInstallRequiresOneScope(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"skill", "install"},
		{"skill", "install", "--global", "--target", t.TempDir()},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(context.Background(), args, &stdout, &stderr)
		if code != exitUsage {
			t.Errorf("Run(%v) exit = %d, stderr = %q", args, code, stderr.String())
		}
	}
}

func TestReadOnlyCommandsReportWriteFailures(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"help"}, {"languages"}, {"version"}} {
		if code := Run(
			context.Background(),
			args,
			failingWriter{},
			io.Discard,
		); code != exitError {
			t.Errorf("Run(%v) exit = %d, want %d", args, code, exitError)
		}
	}
	if code := Run(
		context.Background(),
		[]string{"scan", "--help"},
		io.Discard,
		failingWriter{},
	); code != exitError {
		t.Errorf("Run(scan --help) exit = %d, want %d", code, exitError)
	}
	if code := Run(
		context.Background(),
		[]string{"baseline", "update", "--help"},
		io.Discard,
		failingWriter{},
	); code != exitError {
		t.Errorf("Run(baseline update --help) exit = %d, want %d", code, exitError)
	}
	if code := Run(
		context.Background(),
		[]string{"skill", "install", "--help"},
		io.Discard,
		failingWriter{},
	); code != exitError {
		t.Errorf("Run(skill install --help) exit = %d, want %d", code, exitError)
	}
}
