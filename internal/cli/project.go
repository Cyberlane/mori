package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

const projectPlanVersion = 1

type setupAnswers struct {
	Profile          string   `json:"profile"`
	ComparisonMode   string   `json:"comparison_mode,omitempty"`
	LanguagePairs    []string `json:"language_pairs,omitempty"`
	SQLDialect       string   `json:"sql_dialect,omitempty"`
	EmbeddedSQL      *bool    `json:"embedded_sql,omitempty"`
	Strictness       string   `json:"strictness,omitempty"`
	ExcludeGenerated *bool    `json:"exclude_generated,omitempty"`
	Excludes         []string `json:"exclude,omitempty"`
}

type setupQuestion struct {
	ID          string   `json:"id"`
	Prompt      string   `json:"prompt"`
	Choices     []string `json:"choices,omitempty"`
	Default     any      `json:"default,omitempty"`
	Description string   `json:"description"`
}

type languageInventory struct {
	ID        string `json:"id"`
	Files     int    `json:"files"`
	Fragments string `json:"fragment_kind"`
}

type projectInventory struct {
	SupportedFiles        int                             `json:"supported_files"`
	Languages             []languageInventory             `json:"languages"`
	UnsupportedExtensions []unsupportedExtensionInventory `json:"unsupported_extensions"`
	Warnings              []string                        `json:"warnings"`
}

type unsupportedExtensionInventory struct {
	Extension string `json:"extension"`
	Files     int    `json:"files"`
}

type setupPlan struct {
	Version      int              `json:"version"`
	Command      string           `json:"command"`
	Project      string           `json:"project"`
	ConfigPath   string           `json:"config_path"`
	ConfigExists bool             `json:"config_exists"`
	Inventory    projectInventory `json:"inventory"`
	Current      *config.Settings `json:"current_config,omitempty"`
	Questions    []setupQuestion  `json:"questions"`
	Next         []string         `json:"next_steps"`
}

func runProjectSetup(
	ctx context.Context,
	command string,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)
	agent := false
	answersPath := ""
	apply := false
	dryRun := false
	format := "text"
	flags.BoolVar(&agent, "agent", false, "emit a deterministic, read-only setup plan for a project agent")
	flags.StringVar(&answersPath, "answers", "", "read versioned setup answers from a JSON path or - for stdin")
	flags.BoolVar(&apply, "apply", false, "atomically write the previewed configuration")
	flags.BoolVar(&dryRun, "dry-run", false, "print the proposed configuration without writing it")
	flags.StringVar(&format, "format", format, "agent plan format: json")
	flags.Usage = func() {
		fmt.Fprintf(trackedStderr, "Usage: mori %s [options] [directory]\n", command)
		fmt.Fprintln(trackedStderr, "\nGuide initial Mori configuration without creating baselines, hooks, or remote changes.")
		fmt.Fprintln(trackedStderr, "Use --agent for a machine-readable question plan, then --answers with --dry-run or --apply.")
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
		return usageError(stderr, command+" accepts at most one directory")
	}
	if format != "text" && format != "json" {
		return usageError(stderr, "--format must be text or json")
	}
	if agent && format != "json" {
		format = "json"
	}
	if apply && dryRun {
		return usageError(stderr, "--apply and --dry-run cannot be used together")
	}
	if answersPath == "" && (apply || dryRun) {
		return usageError(stderr, "--apply and --dry-run require --answers")
	}

	directory := "."
	if len(flags.Args()) == 1 {
		directory = flags.Args()[0]
	}
	root, err := projectDirectory(directory)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return exitError
	}
	target := filepath.Join(root, config.FileName)
	current, exists, err := loadExactConfig(target)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return exitError
	}
	if command == "setup" && exists {
		fmt.Fprintf(stderr, "mori: setup: %s already exists; use mori configure to update it\n", displayCLIPath(target))
		return exitError
	}
	if command == "configure" && !exists {
		fmt.Fprintln(stderr, "mori: configure: no .mori.json exists; use mori setup first")
		return exitError
	}
	plan, err := buildSetupPlan(ctx, command, root, target, current, exists)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return exitError
	}
	if answersPath == "" && agent {
		return writeJSON(stdout, plan, stderr)
	}

	var answers setupAnswers
	if answersPath != "" {
		answers, err = readSetupAnswers(answersPath, stdin)
	} else {
		answers, err = promptSetupAnswers(stdin, stdout, current, exists)
	}
	if err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return exitError
	}
	settings, err := applySetupAnswers(current, exists, answers)
	if err != nil {
		return usageError(stderr, err.Error())
	}
	content, err := marshalSettings(settings)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return exitError
	}
	if answersPath != "" && !apply {
		dryRun = true
	}
	if dryRun {
		_, err := stdout.Write(content)
		if err != nil {
			return exitError
		}
		return exitSuccess
	}
	if answersPath == "" {
		fmt.Fprintln(stdout, "\nProposed .mori.json:")
		if _, err := stdout.Write(content); err != nil {
			return exitError
		}
		confirmed, err := promptChoice(bufio.NewReader(stdin), stdout, "Apply this configuration?", "no", []string{"yes", "no"})
		if err != nil {
			fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
			return exitError
		}
		if confirmed != "yes" {
			fmt.Fprintln(stdout, "No changes made.")
			return exitSuccess
		}
	}
	if err := writeAtomicConfig(target, content, exists); err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return exitError
	}
	fmt.Fprintf(stdout, "%s %s\n", map[bool]string{true: "Updated", false: "Created"}[exists], displayCLIPath(target))
	return exitSuccess
}

func projectDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("target must be a directory")
	}
	return absolute, nil
}

func loadExactConfig(path string) (config.Settings, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return config.Settings{}, false, nil
	}
	if err != nil {
		return config.Settings{}, false, fmt.Errorf("inspect config: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return config.Settings{}, false, errors.New("config is not a regular file")
	}
	settings, err := config.Load(path)
	return settings, true, err
}

func buildSetupPlan(ctx context.Context, command, root, target string, current config.Settings, exists bool) (setupPlan, error) {
	options := defaultScanOptions()
	if exists {
		if current.Profile != "" {
			if err := applyScanProfile(&options, current.Profile); err != nil {
				return setupPlan{}, err
			}
		}
		applyConfig(&options, current, root)
	}
	inventory, err := inspectProject(ctx, root, options)
	if err != nil {
		return setupPlan{}, err
	}
	plan := setupPlan{
		Version: projectPlanVersion, Command: command, Project: ".",
		ConfigPath: filepath.ToSlash(filepath.Base(target)), ConfigExists: exists,
		Inventory: inventory,
		Questions: setupQuestions(current, exists),
		Next: []string{
			fmt.Sprintf("mori %s --answers answers.json --dry-run %s", command, "."),
			fmt.Sprintf("mori %s --answers answers.json --apply %s", command, "."),
			"mori doctor .",
		},
	}
	if exists {
		copy := current
		plan.Current = &copy
	}
	return plan, nil
}

func setupQuestions(current config.Settings, exists bool) []setupQuestion {
	profile := profileReview
	if exists && current.Profile != "" {
		profile = current.Profile
	}
	return []setupQuestion{
		{ID: "profile", Prompt: "What is the primary workflow?", Choices: []string{"review", "explore", "sql"}, Default: profile, Description: "review is conservative, explore finds broader candidates, and sql compares queries"},
		{ID: "comparison_mode", Prompt: "Which language relationships should Mori compare?", Choices: []string{"same-language", "cross-language", "all", "pairs"}, Default: "same-language", Description: "pairs requires language_pairs in the answers document"},
		{ID: "strictness", Prompt: "How should incomplete coverage be treated?", Choices: []string{"advisory", "standard", "strict"}, Default: "standard", Description: "strict fails on warnings, parse diagnostics, and incomplete file coverage"},
		{ID: "exclude_generated", Prompt: "Exclude detected generated source?", Default: true, Description: "generated files remain visible in coverage evidence but are not compared"},
		{ID: "exclude", Prompt: "Which additional project-relative globs should be excluded?", Default: []string{}, Description: "vendor and common build directories are already excluded"},
	}
}

func readSetupAnswers(path string, stdin io.Reader) (setupAnswers, error) {
	var reader io.Reader
	var file *os.File
	if path == "-" {
		reader = stdin
	} else {
		info, err := os.Lstat(path)
		if err != nil {
			return setupAnswers{}, fmt.Errorf("read answers: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return setupAnswers{}, errors.New("answers file is not a regular file")
		}
		file, err = os.Open(path)
		if err != nil {
			return setupAnswers{}, fmt.Errorf("read answers: %w", err)
		}
		defer file.Close()
		reader = io.LimitReader(file, 1024*1024+1)
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var answers setupAnswers
	if err := decoder.Decode(&answers); err != nil {
		return setupAnswers{}, fmt.Errorf("decode answers: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return setupAnswers{}, errors.New("decode answers: multiple JSON values")
		}
		return setupAnswers{}, fmt.Errorf("decode answers: %w", err)
	}
	return answers, nil
}

func promptSetupAnswers(stdin io.Reader, stdout io.Writer, current config.Settings, exists bool) (setupAnswers, error) {
	reader := bufio.NewReader(stdin)
	profileDefault := profileReview
	if exists && current.Profile != "" {
		profileDefault = current.Profile
	}
	profile, err := promptChoice(reader, stdout, "Primary workflow", profileDefault, []string{"review", "explore", "sql"})
	if err != nil {
		return setupAnswers{}, err
	}
	mode, err := promptChoice(reader, stdout, "Comparison mode", "same-language", []string{"same-language", "cross-language", "all"})
	if err != nil {
		return setupAnswers{}, err
	}
	strictness, err := promptChoice(reader, stdout, "Coverage policy", "standard", []string{"advisory", "standard", "strict"})
	if err != nil {
		return setupAnswers{}, err
	}
	return setupAnswers{Profile: profile, ComparisonMode: mode, Strictness: strictness, ExcludeGenerated: pointer(true)}, nil
}

func promptChoice(reader *bufio.Reader, stdout io.Writer, prompt, defaultValue string, choices []string) (string, error) {
	fmt.Fprintf(stdout, "%s [%s] (%s): ", prompt, defaultValue, strings.Join(choices, "/"))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value := strings.ToLower(strings.TrimSpace(line))
	if value == "" {
		value = defaultValue
	}
	for _, choice := range choices {
		if value == choice {
			return value, nil
		}
	}
	return "", fmt.Errorf("expected one of %s", strings.Join(choices, ", "))
}

func applySetupAnswers(current config.Settings, exists bool, answers setupAnswers) (config.Settings, error) {
	profile := answers.Profile
	if profile == "" && exists {
		profile = current.Profile
	}
	if profile == "" {
		profile = profileReview
	}
	settings, err := profileSettings(profile)
	if err != nil {
		return config.Settings{}, err
	}
	if exists && (answers.Profile == "" || answers.Profile == current.Profile) {
		settings = current
		settings.Profile = profile
	}
	if answers.ComparisonMode != "" {
		settings.SameLanguageOnly = pointer(false)
		settings.CrossLanguageOnly = pointer(false)
		settings.LanguagePairs = nil
		switch answers.ComparisonMode {
		case "same-language":
			settings.SameLanguageOnly = pointer(true)
		case "cross-language":
			settings.CrossLanguageOnly = pointer(true)
		case "all":
		case "pairs":
			if len(answers.LanguagePairs) == 0 {
				return config.Settings{}, errors.New("comparison_mode pairs requires language_pairs")
			}
			settings.LanguagePairs = append([]string{}, answers.LanguagePairs...)
		default:
			return config.Settings{}, fmt.Errorf("unknown comparison_mode %q", answers.ComparisonMode)
		}
	}
	if answers.SQLDialect != "" {
		if _, err := resolveSQLDialect(answers.SQLDialect); err != nil {
			return config.Settings{}, err
		}
		settings.SQLDialect = answers.SQLDialect
	}
	if answers.EmbeddedSQL != nil {
		settings.EmbeddedSQL = pointer(*answers.EmbeddedSQL)
	}
	if answers.ExcludeGenerated != nil {
		settings.ExcludeGenerated = pointer(*answers.ExcludeGenerated)
	}
	if answers.Excludes != nil {
		if err := source.ValidatePatterns(answers.Excludes); err != nil {
			return config.Settings{}, err
		}
		settings.Excludes = append([]string{}, answers.Excludes...)
	}
	if answers.Strictness != "" {
		switch answers.Strictness {
		case "advisory":
			settings.RequireCoverage = pointer(false)
			settings.FailOnWarning = pointer(false)
			settings.FailOnDiagnostic = pointer(false)
		case "standard":
			settings.RequireCoverage = pointer(true)
			settings.FailOnWarning = pointer(false)
			settings.FailOnDiagnostic = pointer(false)
		case "strict":
			settings.RequireCoverage = pointer(true)
			settings.FailOnWarning = pointer(true)
			settings.FailOnDiagnostic = pointer(true)
		default:
			return config.Settings{}, fmt.Errorf("unknown strictness %q", answers.Strictness)
		}
	}
	options := defaultScanOptions()
	if settings.Profile != "" {
		if err := applyScanProfile(&options, settings.Profile); err != nil {
			return config.Settings{}, err
		}
	}
	applyConfig(&options, settings, ".")
	if err := validateScanOptions(options); err != nil {
		return config.Settings{}, err
	}
	return settings, nil
}

func marshalSettings(settings config.Settings) ([]byte, error) {
	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return append(content, '\n'), nil
}

func writeAtomicConfig(path string, content []byte, replace bool) error {
	directory := filepath.Dir(path)
	var original os.FileInfo
	if replace {
		before, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect config: %w", err)
		}
		if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
			return errors.New("existing config is not a regular file")
		}
		original = before
	}
	temporary, err := os.CreateTemp(directory, ".mori.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		cleanup()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if err := writeConfigFile(temporary, content); err != nil {
		cleanup()
		return err
	}
	if !replace {
		if err := os.Link(temporaryPath, path); err == nil {
			cleanup()
			return syncConfigDirectory(directory)
		} else if errors.Is(err, os.ErrExist) {
			cleanup()
			return fmt.Errorf("%s already exists", config.FileName)
		} else {
			cleanup()
			return fmt.Errorf("install config: %w", err)
		}
	} else {
		before, err := os.Lstat(path)
		if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() ||
			!os.SameFile(original, before) {
			cleanup()
			return errors.New("config changed identity before replacement")
		}
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		cleanup()
		return fmt.Errorf("replace config: %w", err)
	}
	return syncConfigDirectory(directory)
}

func runConfigCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageError(stderr, "config requires show or validate")
	}
	switch args[0] {
	case "show":
		return runConfigShow(args[1:], stdout, stderr)
	case "validate":
		return runConfigValidate(args[1:], stdout, stderr)
	default:
		return usageError(stderr, fmt.Sprintf("unknown config command %q", args[0]))
	}
}

type effectiveConfiguration struct {
	Profile           string   `json:"profile,omitempty"`
	Threshold         float64  `json:"threshold"`
	MinTokens         int      `json:"min_tokens"`
	MaxGroups         int      `json:"max_groups"`
	MaxOccurrences    int      `json:"max_occurrences"`
	MaxPairs          int      `json:"max_pairs"`
	MaxFileBytes      int64    `json:"max_file_bytes"`
	Workers           int      `json:"workers"`
	Format            string   `json:"format"`
	ComparisonDomain  string   `json:"comparison_domain,omitempty"`
	SQLDialect        string   `json:"sql_dialect"`
	EmbeddedSQL       bool     `json:"embedded_sql"`
	Ranking           string   `json:"ranking"`
	SameLanguageOnly  bool     `json:"same_language_only"`
	CrossLanguageOnly bool     `json:"cross_language_only"`
	LanguagePairs     []string `json:"language_pairs"`
	RequireCoverage   bool     `json:"require_coverage"`
	MinFileCoverage   float64  `json:"min_file_coverage"`
	MaxZeroFiles      int      `json:"max_zero_fragment_files"`
	FailOnWarning     bool     `json:"fail_on_warning"`
	FailOnDiagnostic  bool     `json:"fail_on_parse_diagnostic"`
	ExcludeGenerated  bool     `json:"exclude_generated"`
	Excludes          []string `json:"exclude"`
	RespectIgnore     bool     `json:"respect_ignore"`
}

func runConfigShow(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config show", flag.ContinueOnError)
	flags.SetOutput(stderr)
	effective := false
	provenance := false
	flags.BoolVar(&effective, "effective", false, "resolve defaults, profile values, and project settings")
	flags.BoolVar(&provenance, "provenance", false, "include the source of the effective configuration")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitSuccess
		}
		return exitUsage
	}
	if len(flags.Args()) > 1 {
		return usageError(stderr, "config show accepts at most one directory")
	}
	directory := "."
	if len(flags.Args()) == 1 {
		directory = flags.Args()[0]
	}
	root, err := projectDirectory(directory)
	if err != nil {
		fmt.Fprintf(stderr, "mori: config show: %v\n", err)
		return exitError
	}
	settings, path, exists, options, err := projectConfiguration(root)
	if err != nil {
		fmt.Fprintf(stderr, "mori: config show: %v\n", err)
		return exitError
	}
	if !exists {
		fmt.Fprintln(stderr, "mori: config show: no .mori.json found")
		return exitError
	}
	if !effective {
		content, err := marshalSettings(settings)
		if err != nil {
			return exitError
		}
		if _, err := stdout.Write(content); err != nil {
			return exitError
		}
		return exitSuccess
	}
	value := effectiveFromOptions(options)
	if provenance {
		return writeJSON(stdout, map[string]any{"configuration": value, "provenance": map[string]string{"defaults": "built-in", "profile": settings.Profile, "project": displayCLIPath(path)}}, stderr)
	}
	return writeJSON(stdout, value, stderr)
}

func runConfigValidate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitSuccess
		}
		return exitUsage
	}
	if len(flags.Args()) > 1 {
		return usageError(stderr, "config validate accepts at most one directory")
	}
	directory := "."
	if len(flags.Args()) == 1 {
		directory = flags.Args()[0]
	}
	root, err := projectDirectory(directory)
	if err != nil {
		fmt.Fprintf(stderr, "mori: config validate: %v\n", err)
		return exitError
	}
	_, path, exists, _, err := projectConfiguration(root)
	if err != nil {
		fmt.Fprintf(stderr, "mori: config validate: %v\n", err)
		return exitError
	}
	if !exists {
		fmt.Fprintln(stderr, "mori: config validate: no .mori.json found")
		return exitError
	}
	fmt.Fprintf(stdout, "Valid %s\n", displayCLIPath(path))
	return exitSuccess
}

func projectConfiguration(root string) (config.Settings, string, bool, scanOptions, error) {
	path := filepath.Join(root, config.FileName)
	settings, exists, err := loadExactConfig(path)
	options := defaultScanOptions()
	if err != nil || !exists {
		return settings, path, exists, options, err
	}
	if settings.Profile != "" {
		if err := applyScanProfile(&options, settings.Profile); err != nil {
			return settings, path, true, options, err
		}
	}
	applyConfig(&options, settings, root)
	if err := validateScanOptions(options); err != nil {
		return settings, path, true, options, err
	}
	return settings, path, true, options, nil
}

func effectiveFromOptions(options scanOptions) effectiveConfiguration {
	return effectiveConfiguration{Profile: options.profile, Threshold: options.threshold, MinTokens: options.minTokens, MaxGroups: options.maxGroups, MaxOccurrences: options.maxOccurrences, MaxPairs: options.maxPairs, MaxFileBytes: options.maxFileBytes, Workers: options.workers, Format: options.format, ComparisonDomain: options.comparisonDomain, SQLDialect: options.sqlDialect, EmbeddedSQL: options.embeddedSQL, Ranking: options.ranking, SameLanguageOnly: options.sameLanguageOnly, CrossLanguageOnly: options.crossLanguageOnly, LanguagePairs: append([]string{}, options.languagePairs...), RequireCoverage: options.requireCoverage, MinFileCoverage: options.minFileCoverage, MaxZeroFiles: options.maxZeroFiles, FailOnWarning: options.failOnWarning, FailOnDiagnostic: options.failOnDiagnostic, ExcludeGenerated: options.excludeGenerated, Excludes: append([]string{}, options.excludes...), RespectIgnore: options.respectIgnore}
}

func runInspect(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	format, root, options, code := parseProjectReportArgs("inspect", args, stderr)
	if code != exitSuccess {
		return code
	}
	inventory, err := inspectProject(ctx, root, options)
	if err != nil {
		fmt.Fprintf(stderr, "mori: inspect: %v\n", err)
		return exitError
	}
	if format == "json" {
		return writeJSON(stdout, inventory, stderr)
	}
	fmt.Fprintf(stdout, "Supported source files: %d\n", inventory.SupportedFiles)
	for _, item := range inventory.Languages {
		fmt.Fprintf(stdout, "  %-16s %d\n", item.ID, item.Files)
	}
	for _, item := range inventory.UnsupportedExtensions {
		fmt.Fprintf(stdout, "Unsupported %s: %d file(s)\n", item.Extension, item.Files)
	}
	for _, warning := range inventory.Warnings {
		fmt.Fprintf(stdout, "Warning: %s\n", warning)
	}
	return exitSuccess
}

type doctorResult struct {
	Healthy    bool             `json:"healthy"`
	ConfigPath string           `json:"config_path,omitempty"`
	Inventory  projectInventory `json:"inventory"`
	Checks     []doctorCheck    `json:"checks"`
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	format, root, options, code := parseProjectReportArgs("doctor", args, stderr)
	if code != exitSuccess {
		return code
	}
	_, path, exists, _, configErr := projectConfiguration(root)
	inventory, inspectErr := inspectProject(ctx, root, options)
	result := doctorResult{Healthy: true, Inventory: inventory, Checks: []doctorCheck{}}
	if exists {
		result.ConfigPath = displayCLIPath(path)
	}
	if configErr != nil {
		result.Healthy = false
		result.Checks = append(result.Checks, doctorCheck{"configuration", "error", configErr.Error()})
	} else if !exists {
		result.Healthy = false
		result.Checks = append(result.Checks, doctorCheck{"configuration", "missing", "run mori setup"})
	} else {
		result.Checks = append(result.Checks, doctorCheck{"configuration", "ok", "strict JSON and effective options are valid"})
	}
	if inspectErr != nil {
		result.Healthy = false
		result.Checks = append(result.Checks, doctorCheck{"discovery", "error", inspectErr.Error()})
	} else if inventory.SupportedFiles == 0 {
		result.Healthy = false
		result.Checks = append(result.Checks, doctorCheck{"discovery", "empty", "no supported source files discovered"})
	} else {
		result.Checks = append(result.Checks, doctorCheck{"discovery", "ok", fmt.Sprintf("%d supported source files", inventory.SupportedFiles)})
	}
	if len(inventory.Warnings) > 0 {
		result.Healthy = false
		result.Checks = append(result.Checks, doctorCheck{"warnings", "warning", fmt.Sprintf("%d discovery warning(s)", len(inventory.Warnings))})
	} else {
		result.Checks = append(result.Checks, doctorCheck{"warnings", "ok", "none"})
	}
	if format == "json" {
		if code := writeJSON(stdout, result, stderr); code != exitSuccess {
			return code
		}
	} else {
		for _, check := range result.Checks {
			fmt.Fprintf(stdout, "%-14s %-8s %s\n", check.Name, check.Status, check.Detail)
		}
	}
	if !result.Healthy {
		return exitCoverage
	}
	return exitSuccess
}

func parseProjectReportArgs(command string, args []string, stderr io.Writer) (string, string, scanOptions, int) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := "text"
	flags.StringVar(&format, "format", format, "output format: text or json")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", "", scanOptions{}, exitSuccess
		}
		return "", "", scanOptions{}, exitUsage
	}
	if format != "text" && format != "json" {
		return "", "", scanOptions{}, usageError(stderr, "--format must be text or json")
	}
	if len(flags.Args()) > 1 {
		return "", "", scanOptions{}, usageError(stderr, command+" accepts at most one directory")
	}
	directory := "."
	if len(flags.Args()) == 1 {
		directory = flags.Args()[0]
	}
	root, err := projectDirectory(directory)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return "", "", scanOptions{}, exitError
	}
	_, _, exists, options, err := projectConfiguration(root)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %s: %v\n", command, err)
		return "", "", scanOptions{}, exitError
	}
	if !exists {
		options = defaultScanOptions()
	}
	return format, root, options, exitSuccess
}

func inspectProject(ctx context.Context, root string, options scanOptions) (projectInventory, error) {
	domain, domainSet, err := resolveComparisonDomain(options.comparisonDomain)
	if err != nil {
		return projectInventory{}, err
	}
	_ = domain
	dialect, err := resolveSQLDialect(options.sqlDialect)
	if err != nil {
		return projectInventory{}, err
	}
	discovered, err := source.DiscoverContext(ctx, []string{root}, source.Options{Excludes: options.excludes, MaxFileBytes: options.maxFileBytes, IgnoreFiles: options.respectIgnore, ComparisonDomains: domainSet, SQLDialect: dialect, EmbeddedSQL: options.embeddedSQL, ExcludeGenerated: options.excludeGenerated})
	if err != nil {
		return projectInventory{}, err
	}
	counts := map[string]int{}
	specs := map[string]language.Spec{}
	for _, file := range discovered.Files {
		counts[file.Language.ID]++
		specs[file.Language.ID] = file.Language
	}
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := projectInventory{SupportedFiles: len(discovered.Files), Languages: []languageInventory{}, UnsupportedExtensions: []unsupportedExtensionInventory{}, Warnings: []string{}}
	for _, id := range ids {
		result.Languages = append(result.Languages, languageInventory{ID: id, Files: counts[id], Fragments: specs[id].FragmentKind})
	}
	for _, item := range discovered.Unsupported {
		result.UnsupportedExtensions = append(result.UnsupportedExtensions, unsupportedExtensionInventory{item.Extension, item.FileCount})
	}
	for _, warning := range discovered.Warnings {
		result.Warnings = append(result.Warnings, strings.TrimSpace(warning.Path+": "+warning.Message))
	}
	return result, nil
}

func writeJSON(stdout io.Writer, value any, stderr io.Writer) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "mori: write JSON: %v\n", err)
		return exitError
	}
	return exitSuccess
}
