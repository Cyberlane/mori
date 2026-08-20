package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/agentskill"
	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/projectcontract"
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

func TestProjectUpgradeCoordinatesPinAndAgentSkill(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, configFileNameForTest), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mori-version"), []byte("v0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(root, ".agents", "skills", "mori-review-similarity")

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"project", "upgrade", "--check", "--format", "json", root}, &stdout, &stderr)
	if code != exitUpgrade {
		t.Fatalf("check exit = %d, stderr = %q", code, stderr.String())
	}
	var plan projectUpgradePlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode check plan: %v", err)
	}
	if plan.Version != projectUpgradePlanVersion || !plan.Drift || plan.ToolVersion != projectPinnedVersion() {
		t.Fatalf("check plan = %#v", plan)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"project", "upgrade", "--dry-run", "--format", "json", root}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("dry-run exit = %d, stderr = %q", code, stderr.String())
	}
	content, err := os.ReadFile(filepath.Join(root, ".mori-version"))
	if err != nil || string(content) != "v0.1.0\n" {
		t.Fatalf("dry-run changed pin: %q, %v", content, err)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"project", "upgrade", "--apply", "--format", "json", root}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("apply exit = %d, stderr = %q, stdout = %q", code, stderr.String(), stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("decode apply plan: %v", err)
	}
	content, err = os.ReadFile(filepath.Join(root, ".mori-version"))
	if err != nil || string(content) != projectPinnedVersion()+"\n" || plan.Drift || plan.Blocked || len(plan.Backups) != 1 {
		t.Fatalf("applied pin = %q, plan = %#v, err = %v", content, plan, err)
	}
	if content, err := os.ReadFile(filepath.Join(skillPath, "SKILL.md")); err != nil || len(content) == 0 {
		t.Fatalf("skill was not updated: %q, %v", content, err)
	}
}

func TestProjectUpgradeApplyPreservesUnknownSkillAndMakesNoManagedChanges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".mori-version"), []byte(projectPinnedVersion()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(root, ".agents", "skills", "mori-review-similarity")
	if err := os.MkdirAll(skillPath, 0o700); err != nil {
		t.Fatal(err)
	}
	custom := []byte("custom local skill\n")
	if err := os.WriteFile(filepath.Join(skillPath, "SKILL.md"), custom, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"project", "upgrade", "--apply", "--format", "json", root}, &stdout, &stderr)
	if code != exitUpgrade {
		t.Fatalf("apply exit = %d, stderr = %q, stdout = %q", code, stderr.String(), stdout.String())
	}
	var plan projectUpgradePlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil || !plan.Blocked {
		t.Fatalf("blocked plan = %#v, err = %v", plan, err)
	}
	content, err := os.ReadFile(filepath.Join(skillPath, "SKILL.md"))
	if err != nil || !bytes.Equal(content, custom) {
		t.Fatalf("custom skill changed: %q, %v", content, err)
	}
	if _, err := os.Lstat(filepath.Join(root, projectcontract.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blocked apply wrote contract: %v", err)
	}
}

func TestProjectUpgradeApplyReplacesRecordedContractWithBackup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".mori-version"), []byte(projectPinnedVersion()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := agentskill.Install(filepath.Join(root, ".agents", "skills"), false); err != nil {
		t.Fatal(err)
	}
	packageDigest, err := agentskill.PackageDigest()
	if err != nil {
		t.Fatal(err)
	}
	old := desiredProjectContract(projectPinnedVersion(), packageDigest)
	old.ReportSchemaVersion--
	content, err := projectcontract.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(root, projectcontract.FileName)
	if _, err := projectcontract.WriteAtomic(contractPath, content, false); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"project", "upgrade", "--apply", "--format", "json", root}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("apply exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	var plan projectUpgradePlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Drift || plan.Blocked || len(plan.Backups) != 1 {
		t.Fatalf("applied plan = %#v", plan)
	}
	current, exists, err := projectcontract.Load(contractPath)
	if err != nil || !exists || current != desiredProjectContract(projectPinnedVersion(), packageDigest) {
		t.Fatalf("current contract = %#v, %t, %v", current, exists, err)
	}
	backup, exists, err := projectcontract.Load(plan.Backups[0])
	if err != nil || !exists || backup != old {
		t.Fatalf("backup contract = %#v, %t, %v", backup, exists, err)
	}
}

func TestProjectUpgradeApplyRejectsSymlinkedManagedSkillParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available")
	}
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".agents")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mori-version"), []byte("v0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"project", "upgrade", "--apply", "--format", "json", root}, &stdout, &stderr)
	if code != exitUpgrade {
		t.Fatalf("apply exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	var plan projectUpgradePlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil || !plan.Blocked {
		t.Fatalf("blocked plan = %#v, err = %v", plan, err)
	}
	pin, err := os.ReadFile(filepath.Join(root, ".mori-version"))
	if err != nil || string(pin) != "v0.1.0\n" {
		t.Fatalf("blocked apply changed pin: %q, %v", pin, err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("blocked apply wrote outside project: %v, %v", entries, err)
	}
}

func TestProjectUpgradeCheckFailsForManagedConflictWithoutDrift(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("bootstrap exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	skillPath := filepath.Join(root, ".agents", "skills", "mori-review-similarity", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("customized\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code := Run(context.Background(), []string{"project", "upgrade", "--check", "--format", "json", root}, &stdout, &stderr)
	if code != exitUpgrade {
		t.Fatalf("check exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	var plan projectUpgradePlan
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil || !plan.Blocked || plan.Drift {
		t.Fatalf("managed-conflict plan = %#v, err = %v", plan, err)
	}
}

func TestProjectUpgradeFlagsLegacyStagedAutomation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, "Scripts")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "git diff --cached --name-only\narguments=(scan --focus-path file.go)\nmori \"${arguments[@]}\" .\n"
	if err := os.WriteFile(filepath.Join(directory, "mori-pre-commit.sh"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	component := inspectProjectAutomation(root)
	if component.Status != "review" || !strings.Contains(component.Action, "review staged check") ||
		!strings.Contains(component.Detail, "custom staged-path enumeration") {
		t.Fatalf("automation component = %#v", component)
	}
}

func TestProjectUpgradeFlagsLowerLevelStagedAutomation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, "Scripts")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "mori scan --staged --include-focused --require-focused-coverage --fail-on-focused-match .\n"
	if err := os.WriteFile(filepath.Join(directory, "mori-pre-commit.sh"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	component := inspectProjectAutomation(root)
	if component.Status != "review" || !strings.Contains(component.Action, "review staged check") ||
		!strings.Contains(component.Detail, "lower-level staged scan") {
		t.Fatalf("automation component = %#v", component)
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
