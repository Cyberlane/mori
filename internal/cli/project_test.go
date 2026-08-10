package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/config"
)

func TestSetupAgentPlanIsReadOnlyAndProjectRelative(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{"setup", "--agent", root}, strings.NewReader(""), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var plan setupPlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.Version != projectPlanVersion || plan.Command != "setup" || plan.Project != "." ||
		plan.ConfigPath != configFileNameForTest || plan.ConfigExists || plan.Inventory.SupportedFiles != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("plan disclosed absolute project path: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, configFileNameForTest)); !os.IsNotExist(err) {
		t.Fatalf("setup agent wrote config: %v", err)
	}
}

func TestSetupAnswersPreviewApplyAndConfigure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	answers := `{"profile":"review","comparison_mode":"cross-language","strictness":"strict","exclude_generated":true,"exclude":["fixtures/**"]}`
	for _, test := range []struct {
		name      string
		args      []string
		wantWrite bool
	}{
		{"preview", []string{"setup", "--answers", "-", "--dry-run", root}, false},
		{"apply", []string{"setup", "--answers", "-", "--apply", root}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "apply" {
				_ = os.Remove(filepath.Join(root, configFileNameForTest))
			}
			var stdout, stderr bytes.Buffer
			code := RunWithInput(context.Background(), test.args, strings.NewReader(answers), &stdout, &stderr)
			if code != exitSuccess {
				t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
			}
			_, err := os.Stat(filepath.Join(root, configFileNameForTest))
			if test.wantWrite && err != nil {
				t.Fatalf("config missing: %v", err)
			}
			if !test.wantWrite && !os.IsNotExist(err) {
				t.Fatalf("preview wrote config: %v", err)
			}
		})
	}

	configure := `{"profile":"explore","comparison_mode":"all","strictness":"advisory","exclude_generated":false}`
	var stdout, stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{"configure", "--answers", "-", "--apply", root}, strings.NewReader(configure), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("configure exit = %d, stderr = %q", code, stderr.String())
	}
	settings, err := config.Load(filepath.Join(root, configFileNameForTest))
	if err != nil {
		t.Fatal(err)
	}
	if settings.Profile != profileExplore || settings.SameLanguageOnly == nil || *settings.SameLanguageOnly ||
		settings.CrossLanguageOnly == nil || *settings.CrossLanguageOnly || settings.RequireCoverage == nil || *settings.RequireCoverage {
		t.Fatalf("configured settings = %+v", settings)
	}
}

func TestConfigInspectAndDoctor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	content, err := renderProfileConfig(profileReview)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, configFileNameForTest), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"config", "validate", root}, {"config", "show", "--effective", "--provenance", root}, {"inspect", "--format", "json", root}, {"doctor", "--format", "json", root}} {
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), args, &stdout, &stderr); code != exitSuccess {
			t.Fatalf("Run(%v) = %d, stderr = %q", args, code, stderr.String())
		}
		if stdout.Len() == 0 {
			t.Fatalf("Run(%v) wrote no output", args)
		}
	}
}

func TestSetupRefusesSymlinkConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available")
	}
	t.Parallel()
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(out, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(root, configFileNameForTest)); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{"configure", "--agent", root}, strings.NewReader(""), &stdout, &stderr)
	if code != exitError || !strings.Contains(stderr.String(), "not a regular file") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
}

const configFileNameForTest = ".mori.json"
