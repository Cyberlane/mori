package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/agentskill"
	"github.com/Cyberlane/mori/internal/baseline"
	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/config"
)

const projectUpgradePlanVersion = 1

type projectUpgradePlan struct {
	Version     int                       `json:"version"`
	Project     string                    `json:"project"`
	ToolVersion string                    `json:"tool_version"`
	Mode        string                    `json:"mode"`
	Drift       bool                      `json:"drift"`
	Components  []projectUpgradeComponent `json:"components"`
	Backups     []string                  `json:"backups,omitempty"`
}

type projectUpgradeComponent struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Current string `json:"current,omitempty"`
	Target  string `json:"target,omitempty"`
	Action  string `json:"action,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func runProjectUpgrade(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("project upgrade", flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)
	check := false
	dryRun := false
	apply := false
	format := "text"
	flags.BoolVar(&check, "check", false, "report drift without writing; exits 5 when an update is available")
	flags.BoolVar(&dryRun, "dry-run", false, "print the coordinated update plan without writing")
	flags.BoolVar(&apply, "apply", false, "update the project pin and embedded Agent Skill safely")
	flags.StringVar(&format, "format", format, "output format: text or json")
	flags.Usage = func() {
		fmt.Fprintln(trackedStderr, "Usage: mori project upgrade [--check|--dry-run|--apply] [--format text|json] [directory]")
		fmt.Fprintln(trackedStderr, "\nKeep a project's Mori version pin, Agent Skill, configuration, baseline, and automation guidance aligned.")
		fmt.Fprintln(trackedStderr, "The command is local-only and never commits, pushes, installs a CLI, or rewrites project policy.")
		fmt.Fprintln(trackedStderr, "\nOptions:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) && trackedStderr.err == nil {
			return exitSuccess
		}
		if trackedStderr.err != nil {
			return exitError
		}
		return exitUsage
	}
	if len(flags.Args()) > 1 {
		return usageError(stderr, "project upgrade accepts at most one directory")
	}
	selected := 0
	for _, value := range []bool{check, dryRun, apply} {
		if value {
			selected++
		}
	}
	if selected > 1 {
		return usageError(stderr, "--check, --dry-run, and --apply are mutually exclusive")
	}
	if selected == 0 {
		check = true
	}
	if format != "text" && format != "json" {
		return usageError(stderr, "--format must be text or json")
	}
	directory := "."
	if len(flags.Args()) == 1 {
		directory = flags.Args()[0]
	}
	root, err := projectDirectory(directory)
	if err != nil {
		return commandError(stderr, "project upgrade", err)
	}
	mode := "check"
	if dryRun {
		mode = "dry-run"
	} else if apply {
		mode = "apply"
	}
	plan, err := inspectProjectUpgrade(ctx, root, mode)
	if err != nil {
		return commandError(stderr, "project upgrade", err)
	}
	if apply {
		backups, err := applyProjectUpgrade(root, plan)
		if err != nil {
			return commandError(stderr, "project upgrade", err)
		}
		plan, err = inspectProjectUpgrade(ctx, root, mode)
		if err != nil {
			return commandError(stderr, "project upgrade", err)
		}
		plan.Backups = backups
	}
	if format == "json" {
		if code := writeJSON(stdout, plan, stderr); code != exitSuccess {
			return code
		}
	} else if err := writeProjectUpgradeText(stdout, plan); err != nil {
		return exitError
	}
	if check && plan.Drift {
		return exitUpgrade
	}
	if apply && plan.Drift {
		return exitUpgrade
	}
	return exitSuccess
}

func inspectProjectUpgrade(ctx context.Context, root string, mode string) (projectUpgradePlan, error) {
	if err := ctx.Err(); err != nil {
		return projectUpgradePlan{}, err
	}
	targetVersion := projectPinnedVersion()
	plan := projectUpgradePlan{
		Version: projectUpgradePlanVersion, Project: displayCLIPath(root), ToolVersion: targetVersion,
		Mode: mode, Components: make([]projectUpgradeComponent, 0, 5),
	}
	pinPath := filepath.Join(root, ".mori-version")
	pin, err := inspectVersionPin(pinPath, targetVersion)
	if err != nil {
		return projectUpgradePlan{}, err
	}
	plan.Components = append(plan.Components, pin)
	if pin.Status != "current" {
		plan.Drift = true
	}

	skillParent := filepath.Join(root, ".agents", "skills")
	skill, err := agentskill.Inspect(skillParent)
	if err != nil {
		return projectUpgradePlan{}, fmt.Errorf("inspect project Agent Skill: %w", err)
	}
	skillComponent := projectUpgradeComponent{
		Name: "agent-skill", Status: string(skill.Status), Path: displayCLIPath(skill.Path),
		Target: targetVersion, Action: "install the Agent Skill embedded in this Mori binary",
	}
	if skill.Status == agentskill.StatusCurrent {
		skillComponent.Action = ""
	} else {
		plan.Drift = true
	}
	plan.Components = append(plan.Components, skillComponent)

	settings, configPath, configExists, _, configErr := projectConfiguration(root)
	configComponent := projectUpgradeComponent{Name: "configuration", Path: displayCLIPath(configPath)}
	if configErr != nil {
		configComponent.Status = "error"
		configComponent.Detail = configErr.Error()
		configComponent.Action = "repair the strict configuration before relying on project scans"
		plan.Drift = true
	} else if !configExists {
		configComponent.Status = "missing"
		configComponent.Action = "run mori setup; upgrade does not invent project policy"
	} else {
		configComponent.Status = "current"
		configComponent.Detail = "strict configuration is valid"
	}
	plan.Components = append(plan.Components, configComponent)

	baselineComponent := inspectProjectBaseline(root, settings, configExists)
	if baselineComponent.Status == "error" {
		plan.Drift = true
	}
	plan.Components = append(plan.Components, baselineComponent)
	automation := inspectProjectAutomation(root)
	if automation.Status == "review" {
		plan.Drift = true
	}
	plan.Components = append(plan.Components, automation)
	return plan, nil
}

func inspectVersionPin(path string, target string) (projectUpgradeComponent, error) {
	component := projectUpgradeComponent{Name: "version-pin", Path: displayCLIPath(path), Target: target}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		component.Status = "missing"
		component.Action = "create .mori-version"
		return component, nil
	}
	if err != nil {
		return component, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return component, errors.New(".mori-version must be a regular file, not a symlink")
	}
	if info.Size() > 256 {
		return component, errors.New(".mori-version exceeds 256 bytes")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return component, err
	}
	component.Current = strings.TrimSpace(string(content))
	if component.Current == target {
		component.Status = "current"
	} else {
		component.Status = "different"
		component.Action = "update .mori-version"
	}
	return component, nil
}

func inspectProjectBaseline(root string, settings config.Settings, configExists bool) projectUpgradeComponent {
	path := filepath.Join(root, ".mori-baseline.json")
	if configExists && settings.Baseline != "" {
		path = resolveConfigPath(root, settings.Baseline)
	}
	component := projectUpgradeComponent{Name: "baseline", Path: displayCLIPath(path)}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		component.Status = "absent"
		component.Detail = "no accepted-match baseline is configured"
		return component
	} else if err != nil {
		component.Status = "error"
		component.Detail = err.Error()
		return component
	}
	if _, err := baseline.Load(path); err != nil {
		component.Status = "error"
		component.Detail = err.Error()
		component.Action = "repair or migrate the baseline explicitly"
		return component
	}
	component.Status = "current"
	component.Detail = "baseline schema is readable; accepted identities were not changed"
	return component
}

func inspectProjectAutomation(root string) projectUpgradeComponent {
	patterns := []string{
		".github/workflows/*.yml", ".github/workflows/*.yaml", ".pre-commit-config.yaml",
		"lefthook.yml", "lefthook.yaml", "Scripts/*mori*", "scripts/*mori*",
	}
	type detectedFile struct {
		path string
		info os.FileInfo
	}
	files := make([]detectedFile, 0)
	legacyStagedFocus := false
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		for _, match := range matches {
			info, err := os.Lstat(match)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			duplicate := false
			for _, existing := range files {
				if os.SameFile(info, existing.info) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			files = append(files, detectedFile{path: match, info: info})
			if info.Size() <= 1024*1024 {
				content, readErr := os.ReadFile(match)
				if readErr == nil && bytes.Contains(content, []byte("mori")) &&
					bytes.Contains(content, []byte("git diff --cached")) &&
					!bytes.Contains(content, []byte("--staged")) {
					legacyStagedFocus = true
				}
			}
		}
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, displayCLIPath(file.path))
	}
	sort.Strings(paths)
	component := projectUpgradeComponent{Name: "automation", Status: "advisory"}
	if len(paths) == 0 {
		component.Detail = "no conventional Mori automation files detected"
		return component
	}
	component.Detail = fmt.Sprintf("detected %d automation file(s): %s", len(paths), strings.Join(paths, ", "))
	component.Action = "review project-specific automation if release notes change invocation policy"
	if legacyStagedFocus {
		component.Status = "review"
		component.Detail += "; found custom staged-path enumeration without native --staged input"
		component.Action = "review migration to mori scan --staged --include-focused --require-focused-coverage"
	}
	return component
}

func applyProjectUpgrade(root string, plan projectUpgradePlan) ([]string, error) {
	backups := make([]string, 0, 2)
	for _, component := range plan.Components {
		switch component.Name {
		case "version-pin":
			if component.Status == "current" {
				continue
			}
			backup, err := writeVersionPin(root, plan.ToolVersion)
			if err != nil {
				return nil, err
			}
			if backup != "" {
				backups = append(backups, displayCLIPath(backup))
			}
		case "agent-skill":
			if component.Status == "current" {
				continue
			}
			result, err := agentskill.Install(filepath.Join(root, ".agents", "skills"), true)
			if err != nil {
				return nil, err
			}
			if result.BackupPath != "" {
				backups = append(backups, displayCLIPath(result.BackupPath))
			}
		}
	}
	sort.Strings(backups)
	return backups, nil
}

func writeVersionPin(root string, version string) (string, error) {
	path := filepath.Join(root, ".mori-version")
	var previous []byte
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", errors.New(".mori-version must be a regular file, not a symlink")
		}
		previous, err = os.ReadFile(path)
		if err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	target := []byte(version + "\n")
	if bytes.Equal(previous, target) {
		return "", nil
	}
	backup := ""
	if previous != nil {
		file, err := os.CreateTemp(root, ".mori-version.backup-*")
		if err != nil {
			return "", err
		}
		backup = file.Name()
		if _, err := file.Write(previous); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
	}
	temporary, err := os.CreateTemp(root, ".mori-version.tmp-*")
	if err != nil {
		return backup, err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(target); err != nil {
		_ = temporary.Close()
		return backup, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return backup, err
	}
	if err := temporary.Close(); err != nil {
		return backup, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return backup, err
	}
	committed = true
	return backup, nil
}

func projectPinnedVersion() string {
	version := strings.TrimSpace(buildinfo.Current().Version)
	if version == "" || version == "dev" {
		return "dev"
	}
	return "v" + strings.TrimPrefix(version, "v")
}

func writeProjectUpgradeText(writer io.Writer, plan projectUpgradePlan) error {
	if _, err := fmt.Fprintf(writer, "Mori project upgrade (%s -> %s)\n", plan.Mode, plan.ToolVersion); err != nil {
		return err
	}
	for _, component := range plan.Components {
		if _, err := fmt.Fprintf(writer, "%-14s %-10s %s\n", component.Name, component.Status, component.Detail); err != nil {
			return err
		}
		if component.Action != "" {
			if _, err := fmt.Fprintf(writer, "  action: %s\n", component.Action); err != nil {
				return err
			}
		}
	}
	for _, backup := range plan.Backups {
		if _, err := fmt.Fprintf(writer, "backup: %s\n", backup); err != nil {
			return err
		}
	}
	if !plan.Drift {
		_, err := fmt.Fprintln(writer, "Project-managed Mori assets are current.")
		return err
	}
	_, err := fmt.Fprintln(writer, "Project-managed Mori assets need attention.")
	return err
}
