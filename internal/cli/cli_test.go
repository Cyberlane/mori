package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
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
	for _, expected := range []string{"Go", "JavaScript / JSX", "Python", "Rust", "TypeScript / TSX"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("languages output missing %q", expected)
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
	if len(result.Matches) == 0 {
		t.Fatal("JSON report contains no matches")
	}
	if result.Warnings == nil {
		t.Fatal("warnings must encode as an empty array, not null")
	}
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
	if result.TotalMatches != 0 || result.Suppressed == 0 || result.Truncated {
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
