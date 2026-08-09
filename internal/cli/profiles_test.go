package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/model"
)

func TestScanProfilesApplyConservativeDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		profile        string
		threshold      float64
		minTokens      int
		maxGroups      int
		maxOccurrences int
		domain         string
		ranking        string
		same           bool
		require        bool
		generated      bool
	}{
		{"review", profileReview, 0.85, 40, 250, 10, "code", "review", true, true, true},
		{"explore", profileExplore, 0.70, 12, 100, 20, "", "structural", false, false, false},
		{"sql", profileSQL, 0.70, 12, 250, 10, "sql-query", "review", false, true, true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			options, _, code, ok := parseScanOptions("scan", []string{
				"--no-config", "--profile", test.profile,
			}, &stderr, false)
			if !ok || code != exitSuccess {
				t.Fatalf("parse = %t/%d, stderr = %q", ok, code, stderr.String())
			}
			if options.profile != test.profile || options.threshold != test.threshold ||
				options.minTokens != test.minTokens || options.maxGroups != test.maxGroups ||
				options.maxOccurrences != test.maxOccurrences ||
				options.comparisonDomain != test.domain || options.ranking != test.ranking ||
				options.sameLanguageOnly != test.same || options.requireCoverage != test.require ||
				options.excludeGenerated != test.generated {
				t.Fatalf("options = %#v", options)
			}
		})
	}
}

func TestScanProfilePrecedenceAndSelectionOverrides(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, config.FileName)
	if err := os.WriteFile(path, []byte(`{
  "profile": "review",
  "threshold": 0.90,
  "cross_language_only": true
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stderr bytes.Buffer
	configured, _, code, ok := parseScanOptions("scan", []string{
		"--config", path,
	}, &stderr, false)
	if !ok || code != exitSuccess {
		t.Fatalf("configured parse = %t/%d, stderr = %q", ok, code, stderr.String())
	}
	if configured.profile != profileReview || configured.threshold != 0.90 ||
		configured.minTokens != 40 || configured.maxGroups != 250 ||
		configured.maxOccurrences != 10 || !configured.crossLanguageOnly ||
		configured.sameLanguageOnly || configured.comparisonDomain != "code" ||
		configured.ranking != "review" {
		t.Fatalf("configured options = %#v", configured)
	}

	stderr.Reset()
	options, _, code, ok := parseScanOptions("scan", []string{
		"--config", path,
		"--profile", "explore",
		"--threshold", "0.95",
	}, &stderr, false)
	if !ok || code != exitSuccess {
		t.Fatalf("parse = %t/%d, stderr = %q", ok, code, stderr.String())
	}
	if options.profile != profileExplore || options.threshold != 0.95 ||
		!options.crossLanguageOnly || options.sameLanguageOnly ||
		options.comparisonDomain != "" || options.ranking != "structural" {
		t.Fatalf("options = %#v", options)
	}

	stderr.Reset()
	options, _, code, ok = parseScanOptions("scan", []string{
		"--no-config", "--profile", "review", "--cross-language-only",
	}, &stderr, false)
	if !ok || code != exitSuccess || !options.crossLanguageOnly || options.sameLanguageOnly {
		t.Fatalf("review cross-language override = %t/%d/%#v/%q", ok, code, options, stderr.String())
	}
}

func TestRunScanReportsSelectedProfileInSchemaEleven(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--no-config", "--profile", "sql", "--format", "json",
		"../../examples/sql-queries",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if result.SchemaVersion != 11 || result.Configuration.Profile != profileSQL ||
		result.Configuration.ComparisonDomain != "sql-query" ||
		result.Configuration.Ranking != "review" {
		t.Fatalf("profile report = %#v", result)
	}
}

func TestRunInitWritesDeterministicExplicitConfig(t *testing.T) {
	t.Parallel()

	var first bytes.Buffer
	var second bytes.Buffer
	var stderr bytes.Buffer
	args := []string{"init", "--profile", "review", "--stdout"}
	if code := Run(context.Background(), args, &first, &stderr); code != exitSuccess {
		t.Fatalf("first init = %d/%q", code, stderr.String())
	}
	stderr.Reset()
	if code := Run(context.Background(), args, &second, &stderr); code != exitSuccess {
		t.Fatalf("second init = %d/%q", code, stderr.String())
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) || !bytes.HasSuffix(first.Bytes(), []byte("\n")) {
		t.Fatalf("init output was not deterministic: %q / %q", first.String(), second.String())
	}
	var settings config.Settings
	if err := json.Unmarshal(first.Bytes(), &settings); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if settings.Profile != profileReview || settings.Threshold == nil || *settings.Threshold != 0.85 ||
		settings.MinTokens == nil || *settings.MinTokens != 40 ||
		settings.MaxOccurrences == nil || *settings.MaxOccurrences != 10 ||
		settings.SameLanguageOnly == nil || !*settings.SameLanguageOnly ||
		settings.RequireCoverage == nil || !*settings.RequireCoverage ||
		settings.ExcludeGenerated == nil || !*settings.ExcludeGenerated {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestRunInitPreservesExistingConfigUnlessForced(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, config.FileName)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{"init", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("create = %d/%q", code, stderr.String())
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"init", "--profile", "sql", root}, &stdout, &stderr); code != exitError ||
		!strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("preserve = %d/%q", code, stderr.String())
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, unchanged) {
		t.Fatalf("existing config changed: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{
		"init", "--force", "--profile", "sql", root,
	}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("force = %d/%q", code, stderr.String())
	}
	replaced, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(replaced, []byte(`"profile": "sql"`)) {
		t.Fatalf("replacement = %q/%v", replaced, err)
	}
}

func TestRunInitRefusesToReplaceSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("preserve me\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, config.FileName)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{"init", "--force", root}, &stdout, &stderr); code != exitError || !strings.Contains(stderr.String(), "not a regular file") {
		t.Fatalf("force symlink = %d/%q", code, stderr.String())
	}
	content, err := os.ReadFile(outside)
	if err != nil || string(content) != "preserve me\n" {
		t.Fatalf("symlink target = %q/%v", content, err)
	}
}

func TestRunRejectsUnknownProfile(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		args []string
		code int
	}{
		{[]string{"scan", "--no-config", "--profile", "magic", "."}, exitError},
		{[]string{"init", "--profile", "magic", "--stdout"}, exitUsage},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := Run(context.Background(), test.args, &stdout, &stderr); code != test.code {
			t.Fatalf("args %v exit = %d", test.args, code)
		}
		if !strings.Contains(stderr.String(), "unknown profile") {
			t.Fatalf("args %v stderr = %q", test.args, stderr.String())
		}
	}
}
