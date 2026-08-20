package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"github.com/Cyberlane/mori/internal/hookcontract"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/projectcontract"
	"github.com/Cyberlane/mori/internal/reviewreceipt"
)

const projectUpgradePlanVersion = 2
const projectConfigSchemaVersion = 1

type projectUpgradePlan struct {
	Version       int                       `json:"version"`
	Project       string                    `json:"project"`
	ToolVersion   string                    `json:"tool_version"`
	Mode          string                    `json:"mode"`
	Drift         bool                      `json:"drift"`
	Blocked       bool                      `json:"blocked"`
	Preconditions []string                  `json:"preconditions"`
	Components    []projectUpgradeComponent `json:"components"`
	Backups       []string                  `json:"backups,omitempty"`
}

type projectUpgradeComponent struct {
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	Classification string   `json:"classification"`
	Ownership      string   `json:"ownership"`
	Path           string   `json:"path,omitempty"`
	Current        string   `json:"current,omitempty"`
	Target         string   `json:"target,omitempty"`
	Action         string   `json:"action,omitempty"`
	Detail         string   `json:"detail,omitempty"`
	Preconditions  []string `json:"preconditions,omitempty"`
}

func runProjectUpgrade(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("project upgrade", flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)
	check, dryRun, apply := false, false, false
	format := "text"
	flags.BoolVar(&check, "check", false, "report required migration or drift without writing; exits 5 when required work remains")
	flags.BoolVar(&dryRun, "dry-run", false, "print the deterministic plan and preconditions without writing")
	flags.BoolVar(&apply, "apply", false, "atomically update safe Mori-managed assets")
	flags.StringVar(&format, "format", format, "output format: text or json")
	flags.Usage = func() {
		fmt.Fprintln(trackedStderr, "Usage: mori project upgrade [--check|--dry-run|--apply] [--format text|json] [directory]")
		fmt.Fprintln(trackedStderr, "\nInspect or safely migrate a project's Mori contract and managed Agent Skill.")
		fmt.Fprintln(trackedStderr, "The command never rewrites project policy, protected baselines, or project-owned automation.")
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
		backups, applyErr := applyProjectUpgrade(root, plan)
		if applyErr != nil {
			return commandError(stderr, "project upgrade", applyErr)
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
	if check && (plan.Drift || plan.Blocked) {
		return exitUpgrade
	}
	if apply && (plan.Drift || plan.Blocked) {
		return exitUpgrade
	}
	return exitSuccess
}

func inspectProjectUpgrade(ctx context.Context, root string, mode string) (projectUpgradePlan, error) {
	if err := ctx.Err(); err != nil {
		return projectUpgradePlan{}, err
	}
	targetVersion := projectPinnedVersion()
	packageDigest, err := agentskill.PackageDigest()
	if err != nil {
		return projectUpgradePlan{}, err
	}
	targetContract := desiredProjectContract(targetVersion, packageDigest)
	targetContractDigest := projectcontract.Digest(targetContract)
	plan := projectUpgradePlan{Version: projectUpgradePlanVersion, Project: displayCLIPath(root), ToolVersion: targetVersion, Mode: mode, Components: make([]projectUpgradeComponent, 0, 8), Preconditions: make([]string, 0, 8)}
	contractPath := filepath.Join(root, projectcontract.FileName)
	contract, contractExists, contractErr := projectcontract.Load(contractPath)
	contractComponent := projectUpgradeComponent{Name: "project-contract", Path: displayCLIPath(contractPath), Ownership: "mori-managed", Target: targetContractDigest}
	if contractErr != nil {
		contractComponent.Status, contractComponent.Classification = "conflict/manual", "conflict/manual"
		contractComponent.Detail, contractComponent.Action = contractErr.Error(), "repair the tracked contract manually"
	} else if !contractExists {
		contractComponent.Status, contractComponent.Classification = "missing", "required"
		contractComponent.Action, contractComponent.Detail = "create the tracked project contract", "safe bootstrap for projects without a contract"
	} else if contract != targetContract {
		contractComponent.Status, contractComponent.Classification = "outdated", "required"
		contractComponent.Current = projectcontract.Digest(contract)
		contractComponent.Detail, contractComponent.Action = "tracked contract records an older Mori compatibility contract", "update the Mori-managed project contract"
	} else {
		contractComponent.Current = projectcontract.Digest(contract)
		contractComponent.Status, contractComponent.Classification = "current", "current"
		contractComponent.Detail = "tracked contract is valid"
	}
	plan.Components = append(plan.Components, contractComponent)
	pin, err := inspectVersionPin(filepath.Join(root, ".mori-version"), targetVersion)
	pin.Ownership = "mori-managed"
	if err != nil {
		pin.Status, pin.Classification = "conflict/manual", "conflict/manual"
		pin.Detail, pin.Action = err.Error(), "repair the version pin manually"
	} else if pin.Status == "current" {
		pin.Classification = "current"
	} else {
		pin.Classification = "required"
	}
	plan.Components = append(plan.Components, pin)
	skillParent := filepath.Join(root, ".agents", "skills")
	skillParentErr := validateManagedSkillParent(root)
	skill, skillErr := agentskill.Result{}, skillParentErr
	if skillErr == nil {
		skill, skillErr = agentskill.Inspect(skillParent)
	}
	skillPath := filepath.Join(skillParent, agentskill.Name)
	if skill.Path != "" {
		skillPath = skill.Path
	}
	skillComponent := projectUpgradeComponent{Name: "agent-skill", Path: displayCLIPath(skillPath), Target: packageDigest, Ownership: "mori-managed", Action: "install the Agent Skill embedded in this Mori binary"}
	skillDigest := ""
	if skillErr != nil {
		skillComponent.Status, skillComponent.Classification = "conflict/manual", "conflict/manual"
		skillComponent.Detail, skillComponent.Action = skillErr.Error(), "repair or replace the managed skill explicitly"
	} else if skill.Status != agentskill.StatusMissing {
		skillDigest, err = installedSkillDigest(skill.Path)
		if err != nil {
			skillComponent.Status, skillComponent.Classification = "conflict/manual", "conflict/manual"
			skillComponent.Detail, skillComponent.Action = err.Error(), "repair or replace the managed skill explicitly"
		}
	}
	if skillComponent.Classification == "conflict/manual" {
		// Preserve the diagnostic assigned above.
	} else if skill.Status == agentskill.StatusMissing {
		skillComponent.Status, skillComponent.Classification, skillComponent.Detail = "missing", "required", "Agent Skill is not installed"
	} else if skillDigest == packageDigest {
		skillComponent.Status, skillComponent.Classification, skillComponent.Current, skillComponent.Action = "current", "current", skillDigest, ""
		skillComponent.Detail = "installed skill matches the embedded package"
	} else if (contractExists && contractErr == nil && skillDigest == contract.EmbeddedSkill.Digest) ||
		agentskill.IsKnownPriorPackageDigest(skillDigest) {
		skillComponent.Status, skillComponent.Classification, skillComponent.Current = "outdated", "required", skillDigest
		skillComponent.Detail = "installed skill matches a recorded or known prior official package"
	} else {
		skillComponent.Status, skillComponent.Classification, skillComponent.Current = "conflict/manual", "conflict/manual", skillDigest
		skillComponent.Detail = "installed skill differs from the embedded and recorded official packages; local changes are protected"
		skillComponent.Action = "review or replace the skill explicitly; upgrade will not overwrite it"
	}
	plan.Components = append(plan.Components, skillComponent)
	if mode == "gate" {
		finalizeProjectUpgradePlan(&plan)
		return plan, nil
	}
	settings, configPath, configExists, _, configErr := projectConfiguration(root)
	configComponent := projectUpgradeComponent{Name: "configuration", Path: displayCLIPath(configPath), Ownership: "structured-project-policy"}
	if configErr != nil {
		configComponent.Status, configComponent.Classification = "conflict/manual", "conflict/manual"
		configComponent.Detail, configComponent.Action = configErr.Error(), "repair or migrate .mori.json manually; upgrade will not change policy"
	} else if !configExists {
		configComponent.Status, configComponent.Classification = "recommended", "recommended"
		configComponent.Detail, configComponent.Action = "no .mori.json found", "run mori setup if this project needs explicit policy"
	} else {
		configComponent.Status, configComponent.Classification = "current", "current"
		configComponent.Detail = "strict project policy is valid"
	}
	plan.Components = append(plan.Components, configComponent)
	plan.Components = append(plan.Components, inspectProjectPolicyFile(root, ".moriignore"), inspectProjectBaseline(root, settings, configExists), inspectProjectAutomation(root))
	finalizeProjectUpgradePlan(&plan)
	return plan, nil
}

func finalizeProjectUpgradePlan(plan *projectUpgradePlan) {
	for index := range plan.Components {
		component := &plan.Components[index]
		if component.Path != "" && component.Ownership == "mori-managed" {
			precondition := component.Name + ": " + component.Path + " must remain in the observed state"
			component.Preconditions = []string{precondition}
			plan.Preconditions = append(plan.Preconditions, precondition)
		}
		if component.Classification == "required" {
			plan.Drift = true
		}
		if component.Classification == "conflict/manual" && component.Ownership == "mori-managed" {
			plan.Blocked = true
		}
	}
	sort.Strings(plan.Preconditions)
}

func inspectVersionPin(path, target string) (projectUpgradeComponent, error) {
	component := projectUpgradeComponent{Name: "version-pin", Path: displayCLIPath(path), Target: target}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		component.Status, component.Action = "missing", "create .mori-version"
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
		component.Status, component.Action = "different", "update .mori-version"
	}
	return component, nil
}

func inspectProjectPolicyFile(root, name string) projectUpgradeComponent {
	path := filepath.Join(root, name)
	component := projectUpgradeComponent{Name: strings.TrimPrefix(name, "."), Path: displayCLIPath(path), Ownership: "structured-project-policy"}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		component.Status, component.Classification, component.Detail = "current", "current", "policy file is absent"
		return component
	}
	if err != nil {
		component.Status, component.Classification, component.Detail = "conflict/manual", "conflict/manual", err.Error()
		return component
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		component.Status, component.Classification, component.Detail = "conflict/manual", "conflict/manual", "policy file must be a regular file"
		return component
	}
	component.Status, component.Classification, component.Detail = "current", "current", "project policy is preserved"
	return component
}

func inspectProjectBaseline(root string, settings config.Settings, configExists bool) projectUpgradeComponent {
	path := filepath.Join(root, ".mori-baseline.json")
	if configExists && settings.Baseline != "" {
		path = resolveConfigPath(root, settings.Baseline)
	}
	component := projectUpgradeComponent{Name: "baseline", Path: displayCLIPath(path), Ownership: "protected-evidence"}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		component.Status, component.Classification, component.Detail = "current", "current", "no accepted-match baseline is configured"
		return component
	} else if err != nil {
		component.Status, component.Classification, component.Detail = "conflict/manual", "conflict/manual", err.Error()
		return component
	}
	set, err := baseline.Load(path)
	if err != nil {
		component.Status, component.Classification, component.Detail, component.Action = "conflict/manual", "conflict/manual", err.Error(), "repair or migrate the baseline explicitly"
		return component
	}
	if set.Legacy() {
		component.Status, component.Classification, component.Detail, component.Action = "recommended", "recommended", "legacy baseline schema detected; accepted evidence was not changed", "run baseline migrate explicitly after reviewing the migration"
		return component
	}
	component.Status, component.Classification, component.Detail = "current", "current", fmt.Sprintf("baseline schema %d is current; accepted evidence was not changed", baseline.SchemaVersion)
	return component
}

func inspectProjectAutomation(root string) projectUpgradeComponent {
	patterns := []string{".github/workflows/*.yml", ".github/workflows/*.yaml", ".pre-commit-config.yaml", "lefthook.yml", "lefthook.yaml", "Scripts/*mori*", "scripts/*mori*", "AGENTS.md"}
	paths := make([]string, 0)
	seen := make(map[string]struct{})
	legacyStagedFocus, customStagedContract := false, false
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		for _, match := range matches {
			info, err := os.Lstat(match)
			if err == nil && info.Mode().IsRegular() {
				canonical := canonicalPath(match)
				if _, duplicate := seen[canonical]; duplicate {
					continue
				}
				seen[canonical] = struct{}{}
				paths = append(paths, displayCLIPath(match))
				if info.Size() <= 1024*1024 {
					content, readErr := os.ReadFile(match)
					if readErr == nil && bytes.Contains(content, []byte("mori")) && !bytes.Contains(content, []byte("review staged check")) {
						if bytes.Contains(content, []byte("git diff --cached")) && !bytes.Contains(content, []byte("--staged")) {
							legacyStagedFocus = true
						}
						if bytes.Contains(content, []byte("--staged")) {
							customStagedContract = true
						}
					}
				}
			}
		}
	}
	sort.Strings(paths)
	component := projectUpgradeComponent{Name: "automation", Status: "recommended", Classification: "recommended", Ownership: "project-owned-automation"}
	if len(paths) == 0 {
		component.Detail = "no conventional project-owned Mori automation detected"
		return component
	}
	component.Path, component.Detail, component.Action = strings.Join(paths, ", "), fmt.Sprintf("detected %d project-owned automation file(s): %s", len(paths), strings.Join(paths, ", ")), "review automation against the current hook contract; upgrade will not rewrite it"
	if legacyStagedFocus {
		component.Status = "review"
		component.Detail += "; found custom staged-path enumeration without native --staged input"
		component.Action = "review migration to mori review staged check"
	} else if customStagedContract {
		component.Status = "review"
		component.Detail += "; found a lower-level staged scan instead of the canonical staged-review contract"
		component.Action = "review migration to mori review staged check"
	}
	return component
}

func applyProjectUpgrade(root string, plan projectUpgradePlan) ([]string, error) {
	backups := make([]string, 0, 3)
	if plan.Blocked {
		return backups, nil
	}
	components := make(map[string]projectUpgradeComponent, len(plan.Components))
	for _, component := range plan.Components {
		components[component.Name] = component
	}
	for _, name := range []string{"version-pin", "agent-skill", "project-contract"} {
		component, exists := components[name]
		if !exists || component.Classification != "required" {
			continue
		}
		if err := verifyUpgradePrecondition(root, component); err != nil {
			return nil, err
		}
		switch component.Name {
		case "version-pin":
			backup, err := writeVersionPin(root, plan.ToolVersion)
			if err != nil {
				return nil, err
			}
			if backup != "" {
				backups = append(backups, displayCLIPath(backup))
			}
		case "agent-skill":
			result, err := agentskill.Install(filepath.Join(root, ".agents", "skills"), true)
			if err != nil {
				return nil, err
			}
			if result.BackupPath != "" {
				backups = append(backups, displayCLIPath(result.BackupPath))
			}
		case "project-contract":
			packageDigest, err := agentskill.PackageDigest()
			if err != nil {
				return nil, err
			}
			contract := desiredProjectContract(plan.ToolVersion, packageDigest)
			content, err := projectcontract.Marshal(contract)
			if err != nil {
				return nil, err
			}
			backup, err := projectcontract.WriteAtomic(filepath.Join(root, projectcontract.FileName), content, component.Current != "")
			if err != nil {
				return nil, err
			}
			if backup != "" {
				backups = append(backups, displayCLIPath(backup))
			}
		}
	}
	sort.Strings(backups)
	return backups, nil
}

func desiredProjectContract(toolVersion, packageDigest string) projectcontract.Contract {
	return projectcontract.Contract{
		SchemaVersion:        projectcontract.SchemaVersion,
		MoriVersion:          toolVersion,
		EmbeddedSkill:        projectcontract.Artifact{Revision: toolVersion, Digest: packageDigest},
		HookContract:         projectcontract.Artifact{Revision: hookcontract.Revision, Digest: hookcontract.Digest()},
		ConfigSchemaVersion:  projectConfigSchemaVersion,
		ReportSchemaVersion:  model.SchemaVersion,
		ReviewReceiptSchema:  reviewreceipt.SchemaVersion,
		BaselineSchema:       baseline.SchemaVersion,
		NormalizationVersion: normalize.Version,
	}
}

func verifyUpgradePrecondition(root string, component projectUpgradeComponent) error {
	stale := func() error {
		return fmt.Errorf("%s changed after inspection; run project upgrade again", component.Name)
	}
	switch component.Name {
	case "version-pin":
		current, err := inspectVersionPin(filepath.Join(root, ".mori-version"), component.Target)
		if err != nil || current.Status != component.Status || current.Current != component.Current {
			return stale()
		}
	case "agent-skill":
		if err := validateManagedSkillParent(root); err != nil {
			return stale()
		}
		result, err := agentskill.Inspect(filepath.Join(root, ".agents", "skills"))
		if err != nil {
			return stale()
		}
		if component.Status == "missing" {
			if result.Status != agentskill.StatusMissing {
				return stale()
			}
			return nil
		}
		if result.Status == agentskill.StatusMissing {
			return stale()
		}
		digest, err := installedSkillDigest(result.Path)
		if err != nil || digest != component.Current {
			return stale()
		}
	case "project-contract":
		contract, exists, err := projectcontract.Load(filepath.Join(root, projectcontract.FileName))
		if err != nil {
			return stale()
		}
		if component.Status == "missing" {
			if exists {
				return stale()
			}
			return nil
		}
		if !exists || projectcontract.Digest(contract) != component.Current {
			return stale()
		}
	}
	return nil
}

func validateManagedSkillParent(root string) error {
	current := root
	for _, name := range []string{".agents", "skills"} {
		current = filepath.Join(current, name)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("managed skill parent %s must be a real directory", displayCLIPath(current))
		}
	}
	return nil
}

func writeVersionPin(root, version string) (string, error) {
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

func installedSkillDigest(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("installed skill path must be a real directory")
	}
	paths := make([]string, 0)
	err = filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("installed skill contains a symlink: %s", current)
		}
		if info.Mode().IsRegular() {
			relative, err := filepath.Rel(path, current)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, relative := range paths {
		content, err := os.ReadFile(filepath.Join(path, filepath.FromSlash(relative)))
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte(relative))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func writeProjectUpgradeText(writer io.Writer, plan projectUpgradePlan) error {
	if _, err := fmt.Fprintf(writer, "Mori project upgrade (%s -> %s)\n", plan.Mode, plan.ToolVersion); err != nil {
		return err
	}
	for _, component := range plan.Components {
		if _, err := fmt.Fprintf(writer, "%-18s %-14s %-22s %s\n", component.Name, component.Status, component.Ownership, component.Detail); err != nil {
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
	if plan.Blocked {
		message := "A Mori-managed conflict requires manual resolution."
		if plan.Mode == "apply" {
			message = "A Mori-managed conflict requires manual resolution; apply made no managed changes."
		}
		if _, err := fmt.Fprintln(writer, message); err != nil {
			return err
		}
		if !plan.Drift {
			return nil
		}
	}
	if !plan.Drift {
		_, err := fmt.Fprintln(writer, "No required project migration remains.")
		return err
	}
	_, err := fmt.Fprintln(writer, "Required project migration remains.")
	return err
}
