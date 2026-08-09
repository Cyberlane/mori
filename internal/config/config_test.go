package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverFindsNearestParentConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, FileName)
	writeConfig(t, path, `{}`)
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	found, ok, err := Discover(nested)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !ok || found != path {
		t.Fatalf("Discover = %q/%t, want %q/true", found, ok, path)
	}
}

func TestLoadStrictConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), FileName)
	writeConfig(t, path, `{
	"profile": "review",
  "threshold": 0.85,
  "max_groups": 250,
  "comparison_domain": "code",
  "sql_dialect": "postgresql",
	"ranking": "review",
  "same_language_only": false,
  "language_pairs": ["go,typescript"],
  "require_coverage": true,
	"exclude_generated": true,
  "exclude": ["**/*_test.go"],
  "respect_ignore": true
}`)
	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Profile != "review" || settings.Threshold == nil || *settings.Threshold != 0.85 ||
		settings.MaxGroups == nil || *settings.MaxGroups != 250 ||
		settings.ComparisonDomain != "code" ||
		settings.SQLDialect != "postgresql" ||
		settings.Ranking != "review" ||
		settings.SameLanguageOnly == nil || *settings.SameLanguageOnly ||
		len(settings.LanguagePairs) != 1 || settings.RequireCoverage == nil || !*settings.RequireCoverage ||
		settings.ExcludeGenerated == nil || !*settings.ExcludeGenerated ||
		settings.RespectIgnore == nil || !*settings.RespectIgnore {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestLoadRejectsUnknownFieldsAndMultipleValues(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"unknown":  `{"future": true}`,
		"multiple": `{} {}`,
	} {
		name, content := name, content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), FileName)
			writeConfig(t, path, content)
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "decode config") {
				t.Fatalf("Load error = %v, want strict decode error", err)
			}
		})
	}
}

func writeConfig(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
