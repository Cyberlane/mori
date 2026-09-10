package cli

import (
	"bytes"
	"encoding/json"
	"github.com/Cyberlane/mori/internal/feedback"
	"github.com/Cyberlane/mori/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFeedbackCommands(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", base)
	// macOS uses HOME/Library/Application Support rather than XDG_CONFIG_HOME.
	t.Setenv("HOME", base)
	t.Setenv("AppData", base)
	root := t.TempDir()
	for _, action := range []string{"status", "enable", "status", "export", "disable", "clear"} {
		var out, stderr bytes.Buffer
		if code := runFeedback([]string{action, "--root", root}, &out, &stderr); code != 0 {
			t.Fatalf("%s: %d %s", action, code, stderr.String())
		}
		if action == "export" && (!strings.Contains(out.String(), `"samples": []`) || strings.Contains(out.String(), root)) {
			t.Fatal("invalid export")
		}
	}
	var out, stderr bytes.Buffer
	if code := runFeedback([]string{"enable", "--send"}, &out, &stderr); code != exitUsage {
		t.Fatal("unknown option accepted")
	}
}
func TestFeedbackCI(t *testing.T) {
	for _, key := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "TF_BUILD", "JENKINS_URL", "TEAMCITY_VERSION"} {
		t.Setenv(key, "")
	}
	if feedbackCI() {
		t.Fatal("CI inferred")
	}
	t.Setenv("CI", "false")
	if feedbackCI() {
		t.Fatal("false CI enabled")
	}
	t.Setenv("GITHUB_ACTIONS", "true")
	if !feedbackCI() {
		t.Fatal("CI not detected")
	}
}

func TestFeedbackClassificationCommand(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("HOME", base)
	t.Setenv("AppData", base)
	for _, key := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "TF_BUILD", "JENKINS_URL", "TEAMCITY_VERSION"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	var out, stderr bytes.Buffer
	if code := runFeedback([]string{"enable", "--root", root}, &out, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	captureLocalFeedback(root, model.Report{TotalMatchGroups: 1}, time.Second, "completed", "miss")
	if code := runFeedback([]string{"classify", "--root", root, "--classification", "uncertain", "--effort", "under-1m", "--rank", "3"}, &out, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	out.Reset()
	if code := runFeedback([]string{"export", "--root", root}, &out, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(out.String(), `"classification": "uncertain"`) || !strings.Contains(out.String(), `"cache": "miss"`) {
		t.Fatal(out.String())
	}
}

func TestFeedbackSummaryIsOfflineAndRejectsMixedInput(t *testing.T) {
	root := t.TempDir()
	sample := feedback.Measure(model.Report{}, time.Second, "completed")
	raw, err := json.Marshal(feedback.Bundle{SchemaVersion: feedback.SchemaVersion, Samples: []feedback.Sample{sample}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "export.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// An unavailable user config directory must not affect this offline operation.
	t.Setenv("HOME", filepath.Join(root, "missing"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "missing"))
	t.Setenv("AppData", filepath.Join(root, "missing"))
	var out, stderr bytes.Buffer
	if code := runFeedback([]string{"summarize", path}, &out, &stderr); code != exitSuccess || !strings.Contains(out.String(), "Samples: 1") {
		t.Fatalf("%d %s %s", code, out.String(), stderr.String())
	}
	invalid := filepath.Join(root, "report.json")
	if err := os.WriteFile(invalid, []byte(`{"schema_version":21,"source":"private"}`), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := runFeedback([]string{"summarize", path, invalid}, &out, &stderr); code != exitError || out.Len() != 0 || strings.Contains(stderr.String(), root) || strings.Contains(stderr.String(), "private") {
		t.Fatalf("%d %s %s", code, out.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Fatal("summary created consent storage")
	}
}
