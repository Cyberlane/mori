// Package cli implements the mori command-line interface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Cyberlane/mori/internal/analyzer"
	"github.com/Cyberlane/mori/internal/baseline"
	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/diagnostic"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/pathutil"
	"github.com/Cyberlane/mori/internal/report"
	"github.com/Cyberlane/mori/internal/source"
	"github.com/Cyberlane/mori/internal/vcs"
	"github.com/bmatcuk/doublestar/v4"
)

const (
	exitSuccess          = 0
	exitError            = 1
	exitUsage            = 2
	exitFindings         = 3
	exitCoverage         = 4
	maxChangedWorktrees  = 64
	maxFocusedGitPaths   = 100_000
	maxStdinOverlayBytes = 16 * 1024 * 1024
)

// Run executes one CLI invocation and returns its process exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	return RunWithInput(ctx, args, os.Stdin, stdout, stderr)
}

// RunWithInput executes one CLI invocation with an explicit standard input.
// Editors use it to provide a bounded unsaved source overlay.
func RunWithInput(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) int {
	if len(args) == 0 {
		if err := writeRootUsage(stderr); err != nil {
			return exitError
		}
		return exitUsage
	}

	switch args[0] {
	case "scan":
		return runScan(ctx, args[1:], stdin, stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "baseline":
		return runBaseline(ctx, args[1:], stdout, stderr)
	case "languages":
		return runLanguages(args[1:], stdout, stderr)
	case "skill":
		return runSkill(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		if err := writeRootUsage(stdout); err != nil {
			return exitError
		}
		return exitSuccess
	default:
		if _, err := fmt.Fprintf(stderr, "mori: unknown command %q\n\n", args[0]); err != nil {
			return exitError
		}
		if err := writeRootUsage(stderr); err != nil {
			return exitError
		}
		return exitUsage
	}
}

type scanOptions struct {
	profile            string
	excludes           stringList
	languagePairs      stringList
	comparisonDomain   string
	sqlDialect         string
	embeddedSQL        bool
	statementBlocks    bool
	blockStatements    int
	maxBlocksPerFunc   int
	ranking            string
	priorityPaths      stringList
	focusPaths         stringList
	changedSince       string
	changedWorktrees   stringList
	threshold          float64
	minTokens          int
	maxGroups          int
	maxOccurrences     int
	maxPairs           int
	maxFileBytes       int64
	workers            int
	format             string
	sameLanguageOnly   bool
	crossLanguageOnly  bool
	failOnMatch        bool
	failOnFocusedMatch bool
	requireCoverage    bool
	minFileCoverage    float64
	maxZeroFiles       int
	failOnWarning      bool
	failOnDiagnostic   bool
	excludeGenerated   bool
	baselinePath       string
	baselineScope      string
	respectIgnore      bool
	noIgnore           bool
	requestedConfig    string
	configPath         string
	noConfig           bool
	check              bool
	acceptAll          bool
	acceptProfile      bool
	identity           string
	note               string
	noteSet            bool
	classification     string
	classificationSet  bool
	allowedWarnings    stringList
	stdinPath          string
	stdinContent       []byte
}

func defaultScanOptions() scanOptions {
	return scanOptions{
		threshold:        0.70,
		minTokens:        12,
		maxGroups:        100,
		maxOccurrences:   20,
		maxPairs:         5_000_000,
		maxFileBytes:     2 * 1024 * 1024,
		workers:          runtime.GOMAXPROCS(0),
		format:           "text",
		sqlDialect:       language.SQLDialectGeneric,
		blockStatements:  3,
		maxBlocksPerFunc: 64,
		ranking:          analyzer.RankingStructural,
		baselineScope:    string(baseline.ScopeContent),
		respectIgnore:    true,
		maxZeroFiles:     -1,
	}
}

func (options *scanOptions) bindFlags(flags *flag.FlagSet, baselineAction string) {
	flags.StringVar(&options.profile, "profile", options.profile, "scan profile: review, explore, or sql")
	flags.Float64Var(&options.threshold, "threshold", options.threshold, "minimum weighted Jaccard score, from 0 to 1")
	flags.IntVar(&options.minTokens, "min-tokens", options.minTokens, "minimum normalized AST tokens per fragment")
	flags.IntVar(&options.maxGroups, "max-groups", options.maxGroups, "maximum reported content-pair groups; 0 is unlimited")
	flags.IntVar(&options.maxGroups, "max-matches", options.maxGroups, "deprecated alias for --max-groups")
	flags.IntVar(&options.maxOccurrences, "max-occurrences", options.maxOccurrences, "maximum locations retained per fingerprint; 0 is unlimited")
	flags.IntVar(&options.maxPairs, "max-pairs", options.maxPairs, "comparison safety limit; 0 is unlimited")
	flags.Int64Var(&options.maxFileBytes, "max-file-bytes", options.maxFileBytes, "maximum source file size; 0 is unlimited")
	flags.IntVar(&options.workers, "workers", options.workers, "parallel parser workers")
	flags.StringVar(&options.format, "format", options.format, "output format: text, json, or sarif")
	if baselineAction == "" {
		flags.StringVar(
			&options.stdinPath,
			"stdin-path",
			options.stdinPath,
			"analyze bounded stdin as the unsaved content of one discovered source path",
		)
	}
	flags.StringVar(
		&options.comparisonDomain,
		"comparison-domain",
		options.comparisonDomain,
		"scan one comparison domain, such as code or sql-query",
	)
	flags.BoolVar(
		&options.embeddedSQL,
		"embedded-sql",
		options.embeddedSQL,
		"extract direct SQL string arguments from recognized Go database methods; requires --comparison-domain sql-query",
	)
	flags.BoolVar(
		&options.statementBlocks,
		"statement-blocks",
		options.statementBlocks,
		"compare bounded fixed-size statement windows inside functions",
	)
	flags.IntVar(
		&options.blockStatements,
		"block-statements",
		options.blockStatements,
		"number of direct statements in each opt-in block window",
	)
	flags.IntVar(
		&options.maxBlocksPerFunc,
		"max-blocks-per-function",
		options.maxBlocksPerFunc,
		"maximum block windows per function before visible coverage skip",
	)
	flags.Var(
		&options.priorityPaths,
		"priority-path",
		"add review priority for matching paths as GLOB=WEIGHT; repeatable",
	)
	flags.StringVar(
		&options.sqlDialect,
		"sql-dialect",
		options.sqlDialect,
		"SQL parser dialect: generic or postgresql",
	)
	flags.StringVar(
		&options.ranking,
		"ranking",
		options.ranking,
		"group ordering: structural or review",
	)
	flags.BoolVar(
		&options.sameLanguageOnly,
		"same-language-only",
		options.sameLanguageOnly,
		"compare fragments only when their language families match",
	)
	flags.BoolVar(
		&options.crossLanguageOnly,
		"cross-language-only",
		options.crossLanguageOnly,
		"compare fragments only when their language families differ",
	)
	flags.Var(
		&options.languagePairs,
		"language-pair",
		"compare one language ID or family pair, such as go,typescript; repeatable",
	)
	flags.Var(&options.focusPaths, "focus-path", "prioritize groups containing this exact path; repeatable")
	flags.StringVar(
		&options.changedSince,
		"changed-since",
		options.changedSince,
		"prioritize files in the primary Git worktree changed since this local revision",
	)
	flags.Var(
		&options.changedWorktrees,
		"changed-worktree",
		"prioritize changes in an explicit Git worktree as PATH=REVISION; repeatable",
	)
	flags.BoolVar(
		&options.failOnMatch,
		"fail-on-match",
		options.failOnMatch,
		"exit with status 3 when one or more match groups are found",
	)
	flags.BoolVar(
		&options.failOnFocusedMatch,
		"fail-on-focused-match",
		options.failOnFocusedMatch,
		"exit with status 3 when one or more focused match groups are found",
	)
	flags.BoolVar(
		&options.requireCoverage,
		"require-coverage",
		options.requireCoverage,
		"exit with status 4 unless at least one supported file and comparison fragment are analyzed",
	)
	flags.Float64Var(
		&options.minFileCoverage,
		"min-file-coverage",
		options.minFileCoverage,
		"minimum fraction of analyzed files producing fragments, from 0 to 1; 0 disables",
	)
	flags.IntVar(
		&options.maxZeroFiles,
		"max-zero-fragment-files",
		options.maxZeroFiles,
		"maximum analyzed files allowed without fragments; -1 disables",
	)
	flags.BoolVar(
		&options.failOnWarning,
		"fail-on-warning",
		options.failOnWarning,
		"exit with status 4 when any warning is reported",
	)
	flags.BoolVar(
		&options.failOnDiagnostic,
		"fail-on-parse-diagnostic",
		options.failOnDiagnostic,
		"exit with status 4 when any file has parse diagnostics",
	)
	flags.BoolVar(
		&options.excludeGenerated,
		"exclude-generated",
		options.excludeGenerated,
		"exclude files with recognized generated-source header markers",
	)
	flags.StringVar(&options.baselinePath, "baseline", options.baselinePath, "baseline file to load or write")
	flags.StringVar(
		&options.baselineScope,
		"baseline-scope",
		options.baselineScope,
		"baseline identity scope for update: content or path",
	)
	flags.Var(&options.excludes, "exclude", "exclude path glob; repeat for multiple patterns")
	flags.BoolVar(&options.noIgnore, "no-ignore", !options.respectIgnore, "do not read .gitignore or .moriignore files")
	flags.StringVar(&options.requestedConfig, "config", options.requestedConfig, "explicit .mori.json configuration path")
	flags.BoolVar(&options.noConfig, "no-config", options.noConfig, "do not discover or load project configuration")
	if baselineAction == "prune" {
		flags.BoolVar(&options.check, "check", false, "report stale entries without rewriting the baseline")
	}
	if baselineAction == "update" {
		flags.BoolVar(&options.acceptAll, "accept-all", false, "replace the baseline with every candidate in the complete scan")
	}
	if baselineAction == "migrate" {
		flags.BoolVar(&options.acceptProfile, "accept-profile", false, "explicitly accept the active scan profile")
	}
	if baselineAction == "add" {
		flags.StringVar(&options.identity, "identity", "", "content-pair identity to accept")
		flags.StringVar(&options.note, "note", "", "durable human review note; an empty value clears it")
		flags.StringVar(&options.classification, "classification", "", "durable review classification")
	}
	if baselineAction != "" {
		flags.Var(&options.allowedWarnings, "allow-warning", "warning kind permitted for this baseline operation; repeatable")
	}
}

func parseScanOptions(
	command string,
	args []string,
	stderr io.Writer,
	baselineAction string,
) (scanOptions, []string, int, bool) {
	options, err := configuredScanOptions(args)
	if err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "mori: load config: %s\n", diagnostic.Message(err)); writeErr != nil {
			return scanOptions{}, nil, exitError, false
		}
		return scanOptions{}, nil, exitError, false
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)
	options.bindFlags(flags, baselineAction)
	flags.Usage = func() {
		fmt.Fprintf(trackedStderr, "Usage: mori %s [options] [path ...]\n", command)
		fmt.Fprintln(trackedStderr, "\nScan functions and top-level SQL queries for structural similarity.")
		fmt.Fprintln(trackedStderr, "Paths default to the current directory.")
		fmt.Fprintln(trackedStderr, "\nOptions:")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if trackedStderr.err != nil {
				return scanOptions{}, nil, exitError, false
			}
			return scanOptions{}, nil, exitSuccess, false
		}
		if trackedStderr.err != nil {
			return scanOptions{}, nil, exitError, false
		}
		return scanOptions{}, nil, exitUsage, false
	}
	visited := make(map[string]bool)
	flags.Visit(func(value *flag.Flag) {
		visited[value.Name] = true
	})
	options.noteSet = visited["note"]
	options.classificationSet = visited["classification"]
	reconcileCLISelection(&options, visited)
	options.respectIgnore = !options.noIgnore
	if err := validateScanOptions(options); err != nil {
		return scanOptions{}, nil, usageError(stderr, err.Error()), false
	}
	return options, flags.Args(), exitSuccess, true
}

func configuredScanOptions(args []string) (scanOptions, error) {
	request, err := findConfigRequest(args)
	if err != nil {
		return scanOptions{}, err
	}
	options := defaultScanOptions()
	options.noConfig = request.disabled
	options.requestedConfig = request.path
	requestedProfile, err := findProfileRequest(args)
	if err != nil {
		return scanOptions{}, err
	}
	if request.disabled {
		if requestedProfile != "" {
			if err := applyScanProfile(&options, requestedProfile); err != nil {
				return scanOptions{}, err
			}
		}
		return options, nil
	}

	path := request.path
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return scanOptions{}, err
		}
		foundPath, found, err := config.Discover(cwd)
		if err != nil {
			return scanOptions{}, err
		}
		if !found {
			if requestedProfile != "" {
				if err := applyScanProfile(&options, requestedProfile); err != nil {
					return scanOptions{}, err
				}
			}
			return options, nil
		}
		path = foundPath
	}
	settings, err := config.Load(path)
	if err != nil {
		return scanOptions{}, err
	}
	selectedProfile := settings.Profile
	if requestedProfile != "" {
		selectedProfile = requestedProfile
	}
	if selectedProfile != "" {
		if err := applyScanProfile(&options, selectedProfile); err != nil {
			return scanOptions{}, err
		}
	}
	applyConfig(&options, settings, filepath.Dir(path))
	options.configPath = displayCLIPath(path)
	return options, nil
}

func findProfileRequest(args []string) (string, error) {
	var profile string
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--profile":
			if index+1 >= len(args) {
				return "", errors.New("--profile requires a value")
			}
			index++
			profile = args[index]
		case strings.HasPrefix(argument, "--profile="):
			profile = strings.TrimPrefix(argument, "--profile=")
		}
	}
	if profile == "" {
		return "", nil
	}
	resolved, err := resolveScanProfile(profile)
	if err != nil {
		return "", err
	}
	return resolved.name, nil
}

type configRequest struct {
	path     string
	disabled bool
}

func findConfigRequest(args []string) (configRequest, error) {
	request := configRequest{}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--no-config" || argument == "--no-config=true":
			request.disabled = true
		case argument == "--no-config=false":
			request.disabled = false
		case argument == "--config":
			if index+1 >= len(args) {
				return configRequest{}, errors.New("--config requires a path")
			}
			index++
			request.path = args[index]
		case strings.HasPrefix(argument, "--config="):
			request.path = strings.TrimPrefix(argument, "--config=")
		}
	}
	if request.disabled && request.path != "" {
		return configRequest{}, errors.New("--config and --no-config cannot be used together")
	}
	return request, nil
}

func applyConfig(options *scanOptions, settings config.Settings, base string) {
	if settings.Threshold != nil {
		options.threshold = *settings.Threshold
	}
	if settings.MinTokens != nil {
		options.minTokens = *settings.MinTokens
	}
	if settings.MaxGroups != nil {
		options.maxGroups = *settings.MaxGroups
	}
	if settings.MaxOccurrences != nil {
		options.maxOccurrences = *settings.MaxOccurrences
	}
	if settings.MaxPairs != nil {
		options.maxPairs = *settings.MaxPairs
	}
	if settings.MaxFileBytes != nil {
		options.maxFileBytes = *settings.MaxFileBytes
	}
	if settings.Workers != nil {
		options.workers = *settings.Workers
	}
	if settings.Format != nil {
		options.format = *settings.Format
	}
	if settings.CrossLanguageOnly != nil {
		options.crossLanguageOnly = *settings.CrossLanguageOnly
	}
	if settings.SameLanguageOnly != nil {
		options.sameLanguageOnly = *settings.SameLanguageOnly
	}
	reconcileConfiguredSelection(options, settings)
	if settings.FailOnMatch != nil {
		options.failOnMatch = *settings.FailOnMatch
	}
	if settings.RequireCoverage != nil {
		options.requireCoverage = *settings.RequireCoverage
	}
	if settings.MinFileCoverage != nil {
		options.minFileCoverage = *settings.MinFileCoverage
	}
	if settings.MaxZeroFiles != nil {
		options.maxZeroFiles = *settings.MaxZeroFiles
	}
	if settings.FailOnWarning != nil {
		options.failOnWarning = *settings.FailOnWarning
	}
	if settings.FailOnDiagnostic != nil {
		options.failOnDiagnostic = *settings.FailOnDiagnostic
	}
	if settings.ExcludeGenerated != nil {
		options.excludeGenerated = *settings.ExcludeGenerated
	}
	if settings.RespectIgnore != nil {
		options.respectIgnore = *settings.RespectIgnore
	}
	options.noIgnore = !options.respectIgnore
	options.excludes = append(options.excludes, settings.Excludes...)
	options.languagePairs = append(options.languagePairs, settings.LanguagePairs...)
	if settings.ComparisonDomain != "" {
		options.comparisonDomain = settings.ComparisonDomain
	}
	if settings.SQLDialect != "" {
		options.sqlDialect = settings.SQLDialect
	}
	if settings.EmbeddedSQL != nil {
		options.embeddedSQL = *settings.EmbeddedSQL
	}
	if settings.StatementBlocks != nil {
		options.statementBlocks = *settings.StatementBlocks
	}
	if settings.BlockStatements != nil {
		options.blockStatements = *settings.BlockStatements
	}
	if settings.MaxBlocksPerFunc != nil {
		options.maxBlocksPerFunc = *settings.MaxBlocksPerFunc
	}
	if settings.Ranking != "" {
		options.ranking = settings.Ranking
	}
	options.priorityPaths = append(options.priorityPaths, settings.PriorityPaths...)
	if settings.Baseline != "" {
		options.baselinePath = resolveConfigPath(base, settings.Baseline)
	}
	if settings.BaselineScope != "" {
		options.baselineScope = settings.BaselineScope
	}
}

func reconcileConfiguredSelection(options *scanOptions, settings config.Settings) {
	sameSelected := settings.SameLanguageOnly != nil && *settings.SameLanguageOnly
	crossSelected := settings.CrossLanguageOnly != nil && *settings.CrossLanguageOnly
	pairsSelected := len(settings.LanguagePairs) > 0
	if sameSelected && !crossSelected {
		options.crossLanguageOnly = false
		if !pairsSelected {
			options.languagePairs = nil
		}
	}
	if crossSelected && !sameSelected {
		options.sameLanguageOnly = false
		if !pairsSelected {
			options.languagePairs = nil
		}
	}
	if pairsSelected {
		if settings.SameLanguageOnly == nil {
			options.sameLanguageOnly = false
		}
		if settings.CrossLanguageOnly == nil {
			options.crossLanguageOnly = false
		}
	}
}

func reconcileCLISelection(options *scanOptions, visited map[string]bool) {
	sameSelected := visited["same-language-only"] && options.sameLanguageOnly
	crossSelected := visited["cross-language-only"] && options.crossLanguageOnly
	pairsSelected := visited["language-pair"]
	if sameSelected && !crossSelected {
		options.crossLanguageOnly = false
		if !pairsSelected {
			options.languagePairs = nil
		}
	}
	if crossSelected && !sameSelected {
		options.sameLanguageOnly = false
		if !pairsSelected {
			options.languagePairs = nil
		}
	}
	if pairsSelected {
		if !visited["same-language-only"] {
			options.sameLanguageOnly = false
		}
		if !visited["cross-language-only"] {
			options.crossLanguageOnly = false
		}
	}
}

func resolveConfigPath(base string, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func validateScanOptions(options scanOptions) error {
	if options.profile != "" {
		if _, err := resolveScanProfile(options.profile); err != nil {
			return err
		}
	}
	if math.IsNaN(options.threshold) || math.IsInf(options.threshold, 0) ||
		options.threshold <= 0 || options.threshold > 1 {
		return errors.New("--threshold must be greater than 0 and at most 1")
	}
	if options.minTokens < 1 {
		return errors.New("--min-tokens must be at least 1")
	}
	if math.IsNaN(options.minFileCoverage) || math.IsInf(options.minFileCoverage, 0) ||
		options.minFileCoverage < 0 || options.minFileCoverage > 1 {
		return errors.New("--min-file-coverage must be from 0 to 1")
	}
	if options.maxZeroFiles < -1 {
		return errors.New("--max-zero-fragment-files must be -1 or greater")
	}
	allowedWarningKinds := map[string]struct{}{
		"baseline": {}, "coverage": {}, "focus": {}, "other": {}, "parse": {},
	}
	seenWarningKinds := make(map[string]struct{}, len(options.allowedWarnings))
	for _, kind := range options.allowedWarnings {
		if _, ok := allowedWarningKinds[kind]; !ok {
			return fmt.Errorf("unknown --allow-warning kind %q; expected baseline, coverage, focus, other, or parse", kind)
		}
		if _, exists := seenWarningKinds[kind]; exists {
			return fmt.Errorf("duplicate --allow-warning kind %q", kind)
		}
		seenWarningKinds[kind] = struct{}{}
	}
	if options.maxGroups < 0 || options.maxOccurrences < 0 || options.maxPairs < 0 ||
		options.maxFileBytes < 0 {
		return errors.New("maximum values cannot be negative")
	}
	if options.workers < 1 {
		return errors.New("--workers must be at least 1")
	}
	if options.format != "text" && options.format != "json" && options.format != "sarif" {
		return errors.New("--format must be text, json, or sarif")
	}
	if options.stdinPath != "" && options.baselinePath != "" {
		return errors.New("--stdin-path cannot be used with --baseline")
	}
	if _, err := resolveSQLDialect(options.sqlDialect); err != nil {
		return err
	}
	if options.blockStatements < 2 || options.blockStatements > 10 {
		return errors.New("--block-statements must be from 2 to 10")
	}
	if options.maxBlocksPerFunc < 1 || options.maxBlocksPerFunc > 256 {
		return errors.New("--max-blocks-per-function must be from 1 to 256")
	}
	if options.embeddedSQL && options.statementBlocks {
		return errors.New("--embedded-sql and --statement-blocks cannot be used together")
	}
	if _, err := resolveRanking(options.ranking); err != nil {
		return err
	}
	if _, err := parsePriorityPathRules(options.priorityPaths); err != nil {
		return err
	}
	selectionModes := 0
	if options.sameLanguageOnly {
		selectionModes++
	}
	if options.crossLanguageOnly {
		selectionModes++
	}
	if len(options.languagePairs) > 0 {
		selectionModes++
	}
	if selectionModes > 1 {
		return errors.New("--same-language-only, --cross-language-only, and --language-pair are mutually exclusive")
	}
	if options.failOnMatch && options.failOnFocusedMatch {
		return errors.New("--fail-on-match and --fail-on-focused-match cannot be used together")
	}
	if options.failOnFocusedMatch && len(options.focusPaths) == 0 && options.changedSince == "" &&
		len(options.changedWorktrees) == 0 {
		return errors.New("--fail-on-focused-match requires --focus-path, --changed-since, or --changed-worktree")
	}
	if _, err := parseChangedWorktreeSpecs(options.changedWorktrees); err != nil {
		return err
	}
	pairs, err := expandLanguagePairs(options.languagePairs)
	if err != nil {
		return err
	}
	domain, _, err := resolveComparisonDomain(options.comparisonDomain)
	if err != nil {
		return err
	}
	if options.embeddedSQL && domain != "sql-query" {
		return errors.New("--embedded-sql requires --comparison-domain sql-query")
	}
	if options.statementBlocks && domain == "sql-query" {
		return errors.New("--statement-blocks cannot be used with --comparison-domain sql-query")
	}
	if err := validatePairDomain(pairs, domain); err != nil {
		return err
	}
	if err := baseline.ValidateScope(baseline.Scope(options.baselineScope)); err != nil {
		return err
	}
	return source.ValidatePatterns(options.excludes)
}

func runScan(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) int {
	options, paths, code, ok := parseScanOptions("scan", args, stderr, "")
	if !ok {
		return code
	}
	if options.stdinPath != "" {
		content, err := readStdinOverlay(ctx, stdin, options.maxFileBytes)
		if err != nil {
			fmt.Fprintf(stderr, "mori: read --stdin-path overlay: %v\n", err)
			return exitError
		}
		options.stdinContent = content
		options.focusPaths = append(options.focusPaths, options.stdinPath)
	}
	suppress, baselineWarnings, baselineStatus, baselineDigest, err := loadSuppression(options.baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "mori: load baseline: %v\n", err)
		return exitError
	}
	result, err := executeScan(ctx, paths, options, suppress, baselineWarnings)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
	}
	if baselineStatus == "loaded" {
		if baselineDigest != result.Configuration.ScanProfileDigest {
			fmt.Fprintf(
				stderr,
				"mori: load baseline: baseline scan profile %s differs from active profile %s; use matching scan options or run baseline migrate --accept-profile\n",
				baselineDigest,
				result.Configuration.ScanProfileDigest,
			)
			return exitError
		}
		baselineStatus = "compatible"
	}
	result.Configuration.BaselineStatus = baselineStatus
	result.Configuration.BaselineDigest = baselineDigest

	if options.format == "json" {
		err = report.JSON(stdout, result)
	} else if options.format == "sarif" {
		err = report.SARIF(stdout, result)
	} else {
		err = report.Text(stdout, result)
	}
	if err != nil {
		fmt.Fprintf(stderr, "mori: write report: %v\n", err)
		return exitError
	}
	if code := enforceCoveragePolicies(stderr, options, result); code != exitSuccess {
		return code
	}
	if options.failOnMatch && result.TotalMatchGroups > 0 {
		return exitFindings
	}
	if options.failOnFocusedMatch && result.TotalFocusedMatchGroups > 0 {
		return exitFindings
	}
	return exitSuccess
}

func readStdinOverlay(ctx context.Context, reader io.Reader, maxFileBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := int64(maxStdinOverlayBytes)
	if maxFileBytes > 0 && maxFileBytes < limit {
		limit = maxFileBytes
	}
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("stdin content exceeds the %d-byte overlay limit", limit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cloned := make([]byte, len(content))
	copy(cloned, content)
	return cloned, nil
}

func runBaseline(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		return usageError(stderr, "baseline requires add, remove, edit, migrate, update, or prune")
	}
	switch args[0] {
	case "add":
		return runBaselineAdd(ctx, args[1:], stdout, stderr)
	case "remove":
		return runBaselineRemove(args[1:], stdout, stderr)
	case "edit":
		return runBaselineEdit(args[1:], stdout, stderr)
	case "migrate":
		return runBaselineMigrate(ctx, args[1:], stdout, stderr)
	case "update":
		return runBaselineUpdate(ctx, args[1:], stdout, stderr)
	case "prune":
		return runBaselinePrune(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		if err := writeBaselineUsage(stdout); err != nil {
			return exitError
		}
		return exitSuccess
	default:
		return usageError(stderr, "baseline command must be add, remove, edit, migrate, update, or prune")
	}
}

func runBaselineUpdate(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("baseline update", args, stderr, "update")
	if !ok {
		return code
	}
	if len(options.focusPaths) > 0 || options.changedSince != "" || len(options.changedWorktrees) > 0 ||
		options.failOnFocusedMatch {
		return usageError(stderr, "focus options cannot be used with baseline update")
	}
	if options.baselinePath == "" {
		return usageError(stderr, "--baseline is required for baseline update")
	}
	options.maxGroups = 0
	options.maxOccurrences = 0
	result, err := executeScan(ctx, paths, options, nil, nil)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
	}
	if code := enforceCoveragePolicies(stderr, options, result); code != exitSuccess {
		return code
	}
	profile, err := baselineScanProfile(options, result.Configuration.IgnoreEvidence)
	if err != nil {
		return commandError(stderr, "resolve scan profile", err)
	}
	existing, found, err := loadOptionalBaseline(options.baselinePath)
	if err != nil {
		return commandError(stderr, "load baseline", err)
	}
	scope := baseline.Scope(options.baselineScope)
	if found {
		scope = existing.Scope()
	}
	preview := baseline.ReplacementPreview(optionalSet(existing, found), result, scope)
	if _, err := fmt.Fprintf(
		stdout,
		"baseline preview: %s (%d new, %d retained, %d removed); %s\n",
		countLabel(preview.Candidates, "candidate", "candidates"),
		preview.New,
		preview.Retained,
		preview.Removed,
		baselineProfileLabel(existing, found, profile),
	); err != nil {
		return exitError
	}
	if !options.acceptAll {
		if _, err := fmt.Fprintln(stdout, "no file written; rerun with --accept-all after reviewing the preview"); err != nil {
			return exitError
		}
		return exitSuccess
	}
	if code := enforceBaselineCompleteness(stderr, options, result); code != exitSuccess {
		return code
	}
	if found && existing.Legacy() {
		return commandError(stderr, "write baseline", errors.New(
			"legacy baseline must be explicitly migrated before replacement",
		))
	}
	if found && !existing.Compatible(profile) {
		return commandError(stderr, "write baseline", errors.New(
			"baseline scan profile differs from the active scan; run baseline migrate --accept-profile first",
		))
	}
	var previous *baseline.Set
	if found {
		previous = &existing
	}
	if err := baseline.Write(options.baselinePath, result, scope, profile, previous); err != nil {
		fmt.Fprintf(stderr, "mori: write baseline: %v\n", err)
		return exitError
	}
	if _, err := fmt.Fprintf(
		stdout,
		"baseline updated: %q (%s scope, %s covering %s, %s)\n",
		options.baselinePath,
		scope,
		countLabel(result.TotalMatchGroups, "identity", "identities"),
		countLabel(result.TotalLocationPairs, "location pair", "location pairs"),
		countLabel(len(result.Warnings), "warning", "warnings"),
	); err != nil {
		return exitError
	}
	return exitSuccess
}

func runBaselineAdd(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("baseline add", args, stderr, "add")
	if !ok {
		return code
	}
	if options.baselinePath == "" || options.identity == "" {
		return usageError(stderr, "--baseline and --identity are required for baseline add")
	}
	if err := rejectBaselineFocus(options, "add"); err != nil {
		return usageError(stderr, err.Error())
	}
	if len(options.note) > 4096 {
		return usageError(stderr, "--note cannot exceed 4096 bytes")
	}
	if options.classificationSet {
		if err := baseline.ValidateClassification(options.classification); err != nil {
			return usageError(stderr, err.Error())
		}
	}
	set, found, err := loadOptionalBaseline(options.baselinePath)
	if err != nil {
		return commandError(stderr, "load baseline", err)
	}
	options.maxGroups = 0
	options.maxOccurrences = 0
	result, err := executeScan(ctx, paths, options, nil, nil)
	if err != nil {
		return commandError(stderr, "scan", err)
	}
	if code := enforceCoveragePolicies(stderr, options, result); code != exitSuccess {
		return code
	}
	if code := enforceBaselineCompleteness(stderr, options, result); code != exitSuccess {
		return code
	}
	profile, err := baselineScanProfile(options, result.Configuration.IgnoreEvidence)
	if err != nil {
		return commandError(stderr, "resolve scan profile", err)
	}
	if !found {
		set, err = baseline.New(baseline.Scope(options.baselineScope), profile)
		if err != nil {
			return commandError(stderr, "create baseline", err)
		}
	} else if set.Legacy() {
		return commandError(stderr, "add baseline identity", errors.New(
			"legacy baseline must be explicitly migrated before mutation",
		))
	} else if !set.Compatible(profile) {
		return commandError(stderr, "add baseline identity", errors.New(
			"baseline scan profile differs from the active scan",
		))
	}
	var note *string
	if options.noteSet {
		note = &options.note
	}
	var classification *string
	if options.classificationSet {
		classification = &options.classification
	}
	added, updated, err := baseline.Add(
		options.baselinePath,
		set,
		result,
		options.identity,
		note,
		classification,
		profile,
	)
	if err != nil {
		return commandError(stderr, "add baseline identity", err)
	}
	if _, err := fmt.Fprintf(
		stdout,
		"baseline identity accepted: %s (%d new, %d refreshed)\n",
		options.identity,
		added,
		updated,
	); err != nil {
		return exitError
	}
	return exitSuccess
}

func runBaselineMigrate(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("baseline migrate", args, stderr, "migrate")
	if !ok {
		return code
	}
	if options.baselinePath == "" {
		return usageError(stderr, "--baseline is required for baseline migrate")
	}
	if !options.acceptProfile {
		return usageError(stderr, "baseline migrate requires --accept-profile")
	}
	if err := rejectBaselineFocus(options, "migrate"); err != nil {
		return usageError(stderr, err.Error())
	}
	set, err := baseline.Load(options.baselinePath)
	if err != nil {
		return commandError(stderr, "load baseline", err)
	}
	options.maxGroups = 0
	options.maxOccurrences = 0
	result, err := executeScan(ctx, paths, options, nil, nil)
	if err != nil {
		return commandError(stderr, "scan", err)
	}
	if code := enforceCoveragePolicies(stderr, options, result); code != exitSuccess {
		return code
	}
	if code := enforceBaselineCompleteness(stderr, options, result); code != exitSuccess {
		return code
	}
	profile, err := baselineScanProfile(options, result.Configuration.IgnoreEvidence)
	if err != nil {
		return commandError(stderr, "resolve scan profile", err)
	}
	stale := baseline.Stale(set, result)
	if err := baseline.Migrate(options.baselinePath, set, profile); err != nil {
		return commandError(stderr, "migrate baseline", err)
	}
	if _, err := fmt.Fprintf(
		stdout,
		"baseline migrated to schema %d with profile %s (%s retained; %s currently stale)\n",
		baseline.SchemaVersion,
		baseline.Digest(profile),
		countLabel(len(set.Entries()), "entry", "entries"),
		countLabel(len(stale), "entry", "entries"),
	); err != nil {
		return exitError
	}
	return exitSuccess
}

func runBaselineRemove(args []string, stdout io.Writer, stderr io.Writer) int {
	path, identity, _, _, _, code, ok := parseBaselineMetadataFlags("remove", args, stderr)
	if !ok {
		return code
	}
	if path == "" || identity == "" {
		return usageError(stderr, "--baseline and --identity are required for baseline remove")
	}
	set, err := baseline.Load(path)
	if err != nil {
		return commandError(stderr, "load baseline", err)
	}
	removed, err := baseline.Remove(path, set, identity)
	if err != nil {
		return commandError(stderr, "remove baseline identity", err)
	}
	if _, err := fmt.Fprintf(stdout, "baseline identity removed: %s (%s)\n", identity, countLabel(removed, "entry", "entries")); err != nil {
		return exitError
	}
	return exitSuccess
}

func runBaselineEdit(args []string, stdout io.Writer, stderr io.Writer) int {
	path, identity, note, classification, visited, code, ok := parseBaselineMetadataFlags("edit", args, stderr)
	if !ok {
		return code
	}
	if path == "" || identity == "" {
		return usageError(stderr, "--baseline and --identity are required for baseline edit")
	}
	if !visited["note"] && !visited["classification"] {
		return usageError(stderr, "baseline edit requires --note or --classification")
	}
	if len(note) > 4096 {
		return usageError(stderr, "--note cannot exceed 4096 bytes")
	}
	var noteValue *string
	if visited["note"] {
		noteValue = &note
	}
	var classificationValue *string
	if visited["classification"] {
		if err := baseline.ValidateClassification(classification); err != nil {
			return usageError(stderr, err.Error())
		}
		classificationValue = &classification
	}
	set, err := baseline.Load(path)
	if err != nil {
		return commandError(stderr, "load baseline", err)
	}
	updated, err := baseline.Edit(path, set, identity, noteValue, classificationValue)
	if err != nil {
		return commandError(stderr, "edit baseline identity", err)
	}
	if _, err := fmt.Fprintf(stdout, "baseline identity metadata updated: %s (%s)\n", identity, countLabel(updated, "entry", "entries")); err != nil {
		return exitError
	}
	return exitSuccess
}

func parseBaselineMetadataFlags(
	action string,
	args []string,
	stderr io.Writer,
) (string, string, string, string, map[string]bool, int, bool) {
	flags := flag.NewFlagSet("baseline "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var path string
	var identity string
	var note string
	var classification string
	flags.StringVar(&path, "baseline", "", "baseline file to edit")
	flags.StringVar(&identity, "identity", "", "content-pair identity")
	if action == "edit" {
		flags.StringVar(&note, "note", "", "durable human review note; an empty value clears it")
		flags.StringVar(&classification, "classification", "", "durable review classification; an empty value clears it")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", "", "", "", nil, exitSuccess, false
		}
		return "", "", "", "", nil, exitUsage, false
	}
	if len(flags.Args()) > 0 {
		return "", "", "", "", nil, usageError(stderr, "baseline "+action+" does not accept scan paths"), false
	}
	visited := make(map[string]bool)
	flags.Visit(func(value *flag.Flag) { visited[value.Name] = true })
	return path, identity, note, classification, visited, exitSuccess, true
}

func runBaselinePrune(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("baseline prune", args, stderr, "prune")
	if !ok {
		return code
	}
	if len(options.focusPaths) > 0 || options.changedSince != "" || len(options.changedWorktrees) > 0 ||
		options.failOnFocusedMatch {
		return usageError(stderr, "focus options cannot be used with baseline prune")
	}
	if options.baselinePath == "" {
		return usageError(stderr, "--baseline is required for baseline prune")
	}
	set, err := baseline.Load(options.baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "mori: load baseline: %v\n", err)
		return exitError
	}
	if set.Legacy() {
		return commandError(stderr, "prune baseline", errors.New(
			"legacy baseline must be explicitly migrated before pruning",
		))
	}
	options.maxGroups = 0
	options.maxOccurrences = 0
	result, err := executeScan(ctx, paths, options, nil, nil)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
	}
	if code := enforceCoveragePolicies(stderr, options, result); code != exitSuccess {
		return code
	}
	if code := enforceBaselineCompleteness(stderr, options, result); code != exitSuccess {
		return code
	}
	profile, err := baselineScanProfile(options, result.Configuration.IgnoreEvidence)
	if err != nil {
		return commandError(stderr, "resolve scan profile", err)
	}
	if !set.Compatible(profile) {
		return commandError(stderr, "prune baseline", errors.New(
			"baseline scan profile differs from the active scan",
		))
	}
	stale := baseline.Stale(set, result)
	if options.check {
		if len(stale) == 0 {
			_, err := fmt.Fprintln(stdout, "baseline is current")
			if err != nil {
				return exitError
			}
			return exitSuccess
		}
		if _, err := fmt.Fprintf(
			stdout,
			"%s in baseline\n",
			countLabel(len(stale), "stale entry", "stale entries"),
		); err != nil {
			return exitError
		}
		return exitFindings
	}
	if err := baseline.Prune(options.baselinePath, set, result, profile); err != nil {
		fmt.Fprintf(stderr, "mori: prune baseline: %v\n", err)
		return exitError
	}
	if _, err := fmt.Fprintf(
		stdout,
		"baseline pruned: %q (%s scope, %s removed, %s)\n",
		options.baselinePath,
		set.Scope(),
		countLabel(len(stale), "stale entry", "stale entries"),
		countLabel(len(result.Warnings), "warning", "warnings"),
	); err != nil {
		return exitError
	}
	return exitSuccess
}

func countLabel(count int, singular string, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func loadOptionalBaseline(path string) (baseline.Set, bool, error) {
	set, err := baseline.Load(path)
	if err == nil {
		return set, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return baseline.Set{}, false, nil
	}
	return baseline.Set{}, false, err
}

func optionalSet(set baseline.Set, found bool) *baseline.Set {
	if !found {
		return nil
	}
	return &set
}

func baselineProfileLabel(
	set baseline.Set,
	found bool,
	profile baseline.ScanProfile,
) string {
	if !found {
		return "new schema-3 profile " + baseline.Digest(profile)
	}
	if set.Legacy() {
		return "legacy profile unavailable; migration required before mutation"
	}
	if set.Compatible(profile) {
		return "profile compatible: " + set.ProfileDigest()
	}
	return fmt.Sprintf(
		"profile mismatch: baseline %s, active %s",
		set.ProfileDigest(),
		baseline.Digest(profile),
	)
}

func rejectBaselineFocus(options scanOptions, action string) error {
	if len(options.focusPaths) > 0 || options.changedSince != "" ||
		len(options.changedWorktrees) > 0 || options.failOnFocusedMatch {
		return fmt.Errorf("focus options cannot be used with baseline %s", action)
	}
	return nil
}

func enforceBaselineCompleteness(
	stderr io.Writer,
	options scanOptions,
	result model.Report,
) int {
	if result.Truncated {
		if _, err := fmt.Fprintln(stderr, "mori: baseline mutation refused: report is truncated"); err != nil {
			return exitError
		}
		return exitCoverage
	}
	allowed := make(map[string]struct{}, len(options.allowedWarnings))
	for _, kind := range options.allowedWarnings {
		allowed[kind] = struct{}{}
	}
	disallowed := make(map[string]int)
	for _, warning := range result.Warnings {
		kind := warning.Kind
		if kind == "" {
			kind = "other"
		}
		if _, ok := allowed[kind]; !ok {
			disallowed[kind]++
		}
	}
	if len(disallowed) == 0 {
		return exitSuccess
	}
	kinds := make([]string, 0, len(disallowed))
	for kind := range disallowed {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if _, err := fmt.Fprintf(
			stderr,
			"mori: baseline mutation refused: %d disallowed %s warning(s); review and repeat --allow-warning %s if intentional\n",
			disallowed[kind],
			kind,
			kind,
		); err != nil {
			return exitError
		}
	}
	return exitCoverage
}

func hasCoverage(result model.Report) bool {
	return result.Files > 0 && result.Fragments > 0
}

func enforceCoveragePolicies(
	stderr io.Writer,
	options scanOptions,
	result model.Report,
) int {
	failures := make([]string, 0, 4)
	if options.requireCoverage && !hasCoverage(result) {
		message := "no supported source files were discovered"
		if result.Coverage.SupportedFiles > 0 && result.Files == 0 {
			message = "no supported source files were analyzed"
		} else if result.Files > 0 {
			message = "no comparison fragments were extracted"
		}
		failures = append(failures, message)
	}
	if options.minFileCoverage > 0 {
		actual := 0.0
		if result.Coverage.AnalyzedFiles > 0 {
			actual = float64(result.Coverage.FragmentFiles) /
				float64(result.Coverage.AnalyzedFiles)
		}
		if result.Coverage.AnalyzedFiles == 0 || actual < options.minFileCoverage {
			failures = append(failures, fmt.Sprintf(
				"file coverage %d/%d (%.1f%%) is below %.1f%%",
				result.Coverage.FragmentFiles,
				result.Coverage.AnalyzedFiles,
				actual*100,
				options.minFileCoverage*100,
			))
		}
	}
	if options.maxZeroFiles >= 0 &&
		result.Coverage.ZeroFragmentFiles > options.maxZeroFiles {
		failures = append(failures, fmt.Sprintf(
			"%d zero-fragment file(s) exceed the allowed maximum of %d",
			result.Coverage.ZeroFragmentFiles,
			options.maxZeroFiles,
		))
	}
	if options.failOnWarning && result.Coverage.WarningCount > 0 {
		failures = append(failures, fmt.Sprintf(
			"%d warning(s) were reported across %d file(s)",
			result.Coverage.WarningCount,
			result.Coverage.WarningFiles,
		))
	}
	if options.failOnDiagnostic && result.Coverage.ParseDiagnosticFiles > 0 {
		failures = append(failures, fmt.Sprintf(
			"%d parse diagnostic(s) were reported across %d file(s)",
			result.Coverage.ParseDiagnosticCount,
			result.Coverage.ParseDiagnosticFiles,
		))
	}
	if len(failures) == 0 {
		return exitSuccess
	}
	for _, failure := range failures {
		if _, err := fmt.Fprintf(stderr, "mori: coverage policy not met: %s\n", failure); err != nil {
			return exitError
		}
	}
	return exitCoverage
}

func baselineScanProfile(
	options scanOptions,
	ignoreEvidence []model.IgnoreFileEvidence,
) (baseline.ScanProfile, error) {
	domain, _, err := resolveComparisonDomain(options.comparisonDomain)
	if err != nil {
		return baseline.ScanProfile{}, err
	}
	dialect, err := resolveSQLDialect(options.sqlDialect)
	if err != nil {
		return baseline.ScanProfile{}, err
	}
	resolvedPairs, err := expandLanguagePairs(options.languagePairs)
	if err != nil {
		return baseline.ScanProfile{}, err
	}
	pairs := make([]string, 0, len(resolvedPairs))
	for _, pair := range resolvedPairs {
		left, right := pair.Left, pair.Right
		if right < left {
			left, right = right, left
		}
		pairs = append(pairs, left+","+right)
	}
	excludes := append([]string{}, options.excludes...)
	sort.Strings(pairs)
	pairs = compactStrings(pairs)
	sort.Strings(excludes)
	excludes = compactStrings(excludes)
	ignoreFiles := make([]baseline.IgnoreFile, 0, len(ignoreEvidence))
	for _, evidence := range ignoreEvidence {
		ignoreFiles = append(ignoreFiles, baseline.IgnoreFile{
			Path: evidence.Path, Digest: evidence.Digest,
		})
	}
	return baseline.ScanProfile{
		Threshold:         options.threshold,
		MinTokens:         options.minTokens,
		MaxPairs:          options.maxPairs,
		MaxFileBytes:      options.maxFileBytes,
		ComparisonDomain:  domain,
		SQLDialect:        dialect,
		EmbeddedSQL:       options.embeddedSQL,
		StatementBlocks:   options.statementBlocks,
		BlockStatements:   options.blockStatements,
		MaxBlocksPerFunc:  options.maxBlocksPerFunc,
		SameLanguageOnly:  options.sameLanguageOnly,
		CrossLanguageOnly: options.crossLanguageOnly,
		LanguagePairs:     pairs,
		ExcludeGenerated:  options.excludeGenerated,
		Excludes:          excludes,
		RespectIgnore:     options.respectIgnore,
		IgnoreFiles:       ignoreFiles,
		RequireCoverage:   options.requireCoverage,
		MinFileCoverage:   options.minFileCoverage,
		MaxZeroFiles:      options.maxZeroFiles,
		FailOnWarning:     options.failOnWarning,
		FailOnDiagnostic:  options.failOnDiagnostic,
	}, nil
}

func loadSuppression(
	path string,
) (
	func(string, model.Location, model.Location) bool,
	[]model.Warning,
	string,
	string,
	error,
) {
	if path == "" {
		return nil, nil, "", "", nil
	}
	set, err := baseline.Load(path)
	if err != nil {
		return nil, nil, "", "", err
	}
	if set.Legacy() {
		return set.Match, []model.Warning{{
			Kind:    "baseline",
			Path:    displayCLIPath(path),
			Message: "legacy baseline has no scan-profile evidence; run baseline migrate before mutation or strict gating",
		}}, "legacy", "", nil
	}
	return set.Match, nil, "loaded", set.ProfileDigest(), nil
}

func executeScan(
	ctx context.Context,
	paths []string,
	options scanOptions,
	suppress func(string, model.Location, model.Location) bool,
	initialWarnings []model.Warning,
) (model.Report, error) {
	domain, domainSet, err := resolveComparisonDomain(options.comparisonDomain)
	if err != nil {
		return model.Report{}, err
	}
	sqlDialect, err := resolveSQLDialect(options.sqlDialect)
	if err != nil {
		return model.Report{}, err
	}
	ranking, err := resolveRanking(options.ranking)
	if err != nil {
		return model.Report{}, err
	}
	priorityPaths, err := parsePriorityPathRules(options.priorityPaths)
	if err != nil {
		return model.Report{}, err
	}
	discovered, err := source.DiscoverContext(ctx, paths, source.Options{
		Excludes:          options.excludes,
		MaxFileBytes:      options.maxFileBytes,
		IgnoreFiles:       options.respectIgnore,
		ComparisonDomains: domainSet,
		SQLDialect:        sqlDialect,
		EmbeddedSQL:       options.embeddedSQL,
		ExcludeGenerated:  options.excludeGenerated,
	})
	if err != nil {
		return model.Report{}, fmt.Errorf("discover source: %w", err)
	}
	if options.stdinPath != "" {
		overlayPath, err := filepath.Abs(options.stdinPath)
		if err != nil {
			return model.Report{}, fmt.Errorf("resolve --stdin-path: %w", err)
		}
		overlayCanonical := canonicalPath(overlayPath)
		matched := false
		for index := range discovered.Files {
			if canonicalPath(discovered.Files[index].Path) != overlayCanonical {
				continue
			}
			discovered.Files[index].Content = make([]byte, len(options.stdinContent))
			copy(discovered.Files[index].Content, options.stdinContent)
			if spec, ok := language.DetectWithSourcePrefix(
				discovered.Files[index].Path,
				sqlDialect,
				options.stdinContent,
			); ok {
				discovered.Files[index].Language = spec
				discovered.Files[index].AnalysisDomain = spec.ComparisonDomain
			}
			matched = true
			break
		}
		if !matched {
			return model.Report{}, fmt.Errorf(
				"--stdin-path %q is not one discovered supported source file",
				displayCLIPath(overlayPath),
			)
		}
	}
	profile, err := baselineScanProfile(options, discovered.IgnoreEvidence)
	if err != nil {
		return model.Report{}, err
	}
	changes, worktreeMode, err := resolveChangedWorktrees(
		ctx,
		discovered.Files,
		options.changedSince,
		options.changedWorktrees,
	)
	if err != nil {
		return model.Report{}, fmt.Errorf("resolve changed files: %w", err)
	}
	focus, focusSet, focusWarnings, err := resolveFocus(
		options.focusPaths,
		changes,
		worktreeMode,
		discovered.Files,
	)
	if err != nil {
		return model.Report{}, fmt.Errorf("resolve focus: %w", err)
	}
	discovered.Warnings = append(discovered.Warnings, initialWarnings...)
	discovered.Warnings = append(discovered.Warnings, focusWarnings...)
	pairs, err := expandLanguagePairs(options.languagePairs)
	if err != nil {
		return model.Report{}, err
	}
	result, err := analyzer.Analyze(ctx, discovered.Files, discovered.Warnings, analyzer.Options{
		Threshold:         options.threshold,
		MinTokens:         options.minTokens,
		MaxGroups:         options.maxGroups,
		MaxOccurrences:    options.maxOccurrences,
		MaxPairs:          options.maxPairs,
		Workers:           options.workers,
		SameLanguageOnly:  options.sameLanguageOnly,
		CrossLanguageOnly: options.crossLanguageOnly,
		LanguagePairs:     pairs,
		FocusPaths:        focusSet,
		FocusActive:       focus != nil,
		Suppress:          suppress,
		ExcludedCoverage:  discovered.Excluded,
		Unsupported:       discovered.Unsupported,
		Ranking:           ranking,
		PriorityPaths:     priorityPaths,
		EmbeddedSQL:       options.embeddedSQL,
		SQLDialect:        sqlDialect,
		StatementBlocks:   options.statementBlocks,
		BlockStatements:   options.blockStatements,
		MaxBlocksPerFunc:  options.maxBlocksPerFunc,
	})
	if err != nil {
		return result, err
	}
	result.Configuration = model.EffectiveConfig{
		Profile:           options.profile,
		ConfigPath:        options.configPath,
		IgnoreFiles:       discovered.IgnoreFiles,
		IgnoreEvidence:    append([]model.IgnoreFileEvidence{}, discovered.IgnoreEvidence...),
		RespectIgnore:     options.respectIgnore,
		ExcludeGenerated:  options.excludeGenerated,
		Excludes:          append([]string{}, options.excludes...),
		MinTokens:         options.minTokens,
		MaxGroups:         options.maxGroups,
		MaxOccurrences:    options.maxOccurrences,
		MaxPairs:          options.maxPairs,
		MaxFileBytes:      options.maxFileBytes,
		ComparisonDomain:  domain,
		SQLDialect:        sqlDialect,
		EmbeddedSQL:       options.embeddedSQL,
		StatementBlocks:   options.statementBlocks,
		BlockStatements:   options.blockStatements,
		MaxBlocksPerFunc:  options.maxBlocksPerFunc,
		Ranking:           ranking,
		PriorityPaths:     append(make([]model.PriorityPathRule, 0, len(priorityPaths)), priorityPaths...),
		SameLanguageOnly:  options.sameLanguageOnly,
		CrossLanguageOnly: options.crossLanguageOnly,
		LanguagePairs:     append([]string{}, options.languagePairs...),
		RequireCoverage:   options.requireCoverage,
		MinFileCoverage:   options.minFileCoverage,
		MaxZeroFiles:      options.maxZeroFiles,
		FailOnWarning:     options.failOnWarning,
		FailOnDiagnostic:  options.failOnDiagnostic,
		BaselinePath:      displayOptionalPath(options.baselinePath),
		ScanProfileDigest: baseline.Digest(profile),
		StdinPath:         displayOptionalPath(options.stdinPath),
		Focus:             focus,
	}
	result.Tool = buildinfo.Current()
	result.Tool.NormalizationVersion = normalize.Version
	sort.Strings(result.Configuration.Excludes)
	sort.Strings(result.Configuration.LanguagePairs)
	return result, nil
}

const maxPriorityPathRules = 32

func parsePriorityPathRules(values []string) ([]model.PriorityPathRule, error) {
	if len(values) > maxPriorityPathRules {
		return nil, fmt.Errorf("at most %d --priority-path rules are allowed", maxPriorityPathRules)
	}
	rules := make([]model.PriorityPathRule, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		separator := strings.LastIndexByte(value, '=')
		if separator <= 0 || separator == len(value)-1 {
			return nil, fmt.Errorf("invalid --priority-path %q; expected GLOB=WEIGHT", value)
		}
		pattern := strings.TrimSpace(value[:separator])
		weight, err := strconv.Atoi(strings.TrimSpace(value[separator+1:]))
		if pattern == "" || err != nil || weight < 1 || weight > 100 {
			return nil, fmt.Errorf("invalid --priority-path %q; weight must be an integer from 1 to 100", value)
		}
		if len(pattern) > 512 {
			return nil, fmt.Errorf("invalid --priority-path %q; glob exceeds 512 bytes", value)
		}
		if _, exists := seen[pattern]; exists {
			return nil, fmt.Errorf("duplicate --priority-path pattern %q", pattern)
		}
		if _, err := doublestar.Match(pattern, "path"); err != nil {
			return nil, fmt.Errorf("invalid --priority-path pattern %q: %w", pattern, err)
		}
		seen[pattern] = struct{}{}
		rules = append(rules, model.PriorityPathRule{Pattern: pattern, Priority: weight})
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Pattern != rules[j].Pattern {
			return rules[i].Pattern < rules[j].Pattern
		}
		return rules[i].Priority < rules[j].Priority
	})
	return rules, nil
}

func resolveRanking(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case analyzer.RankingStructural, analyzer.RankingReview:
		return value, nil
	default:
		return "", fmt.Errorf(
			"unknown --ranking %q; expected structural or review",
			value,
		)
	}
}

func resolveSQLDialect(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range language.SQLDialects() {
		if value == candidate {
			return candidate, nil
		}
	}
	return "", fmt.Errorf(
		"unknown --sql-dialect %q; expected one of %s",
		value,
		strings.Join(language.SQLDialects(), ", "),
	)
}

type changedWorktreeSpec struct {
	Path     string
	Revision string
}

func parseChangedWorktreeSpecs(values []string) ([]changedWorktreeSpec, error) {
	if len(values) > maxChangedWorktrees {
		return nil, fmt.Errorf(
			"--changed-worktree count exceeded the %d worktree safety limit",
			maxChangedWorktrees,
		)
	}
	specs := make([]changedWorktreeSpec, 0, len(values))
	for _, value := range values {
		path, revision, ok := strings.Cut(value, "=")
		path = strings.TrimSpace(path)
		revision = strings.TrimSpace(revision)
		if !ok || path == "" || revision == "" {
			return nil, fmt.Errorf(
				"invalid --changed-worktree %q; expected PATH=REVISION",
				value,
			)
		}
		specs = append(specs, changedWorktreeSpec{Path: path, Revision: revision})
	}
	return specs, nil
}

func resolveChangedWorktrees(
	ctx context.Context,
	files []source.File,
	primaryRevision string,
	values []string,
) ([]vcs.Changes, bool, error) {
	specs, err := parseChangedWorktreeSpecs(values)
	if err != nil {
		return nil, false, err
	}
	if primaryRevision == "" && len(specs) == 0 {
		return nil, false, nil
	}
	type resolvedSpec struct {
		changedWorktreeSpec
		root  string
		files int
	}
	resolved := make([]resolvedSpec, 0, len(specs))
	seenRoots := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		absolute, err := filepath.Abs(spec.Path)
		if err != nil {
			return nil, true, fmt.Errorf("resolve --changed-worktree path: %w", err)
		}
		root, err := filepath.EvalSymlinks(filepath.Clean(absolute))
		if err != nil {
			return nil, true, fmt.Errorf("resolve --changed-worktree %q: %w", spec.Path, err)
		}
		if _, exists := seenRoots[root]; exists {
			return nil, true, fmt.Errorf("duplicate --changed-worktree root %q", displayCLIPath(root))
		}
		seenRoots[root] = struct{}{}
		resolved = append(resolved, resolvedSpec{changedWorktreeSpec: spec, root: root})
	}
	sort.Slice(resolved, func(left, right int) bool {
		if len(resolved[left].root) != len(resolved[right].root) {
			return len(resolved[left].root) > len(resolved[right].root)
		}
		return resolved[left].root < resolved[right].root
	})

	remaining := make([]string, 0, len(files))
	for _, file := range files {
		path := canonicalPath(file.Path)
		assigned := false
		for index := range resolved {
			if pathutil.Within(resolved[index].root, path) {
				resolved[index].files++
				assigned = true
				break
			}
		}
		if !assigned {
			remaining = append(remaining, file.Path)
		}
	}
	for _, spec := range resolved {
		if spec.files == 0 && len(files) > 0 {
			return nil, true, fmt.Errorf(
				"--changed-worktree root %q contains no discovered source files",
				displayCLIPath(spec.root),
			)
		}
	}
	changes := make([]vcs.Changes, 0, len(resolved)+1)
	totalPaths := 0
	if primaryRevision != "" {
		if len(remaining) == 0 {
			return nil, true, errors.New(
				"--changed-since has no discovered files outside --changed-worktree roots",
			)
		}
		change, err := vcs.ResolveChanged(ctx, remaining, primaryRevision)
		if err != nil {
			return nil, true, err
		}
		totalPaths += len(change.ChangedPaths) + len(change.DeletedPaths)
		changes = append(changes, change)
	} else if len(remaining) > 0 {
		return nil, true, fmt.Errorf(
			"%d discovered source file(s) are outside the explicit --changed-worktree roots",
			len(remaining),
		)
	}
	for _, spec := range resolved {
		change, err := vcs.ResolveChangedAtRoot(ctx, spec.root, spec.Revision)
		if err != nil {
			return nil, true, fmt.Errorf("resolve --changed-worktree %q: %w", spec.Path, err)
		}
		totalPaths += len(change.ChangedPaths) + len(change.DeletedPaths)
		if totalPaths > maxFocusedGitPaths {
			return nil, true, fmt.Errorf(
				"Git changed path count exceeded the %d path safety limit across worktrees",
				maxFocusedGitPaths,
			)
		}
		changes = append(changes, change)
	}
	sort.Slice(changes, func(left, right int) bool {
		return changes[left].Root < changes[right].Root
	})
	return changes, len(specs) > 0, nil
}

func displayWorktreePath(root string, path string) string {
	if root == "." {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(root), filepath.FromSlash(path)))
}

func resolveFocus(
	values []string,
	changes []vcs.Changes,
	worktreeMode bool,
	files []source.File,
) (*model.FocusConfig, map[string]struct{}, []model.Warning, error) {
	if len(values) == 0 && len(changes) == 0 {
		return nil, nil, nil, nil
	}
	fileByPath := make(map[string]source.File, len(files))
	for _, file := range files {
		fileByPath[canonicalPath(file.Path)] = file
	}
	explicit := make([]string, 0, len(values))
	matched := make(map[string]struct{})
	missing := 0
	for _, value := range values {
		cwdAbsolute, err := filepath.Abs(value)
		if err != nil {
			return nil, nil, nil, err
		}
		candidates := []string{cwdAbsolute}
		if len(changes) == 1 && !filepath.IsAbs(value) {
			candidates = append(candidates, filepath.Join(changes[0].Root, filepath.FromSlash(value)))
		}
		found := false
		for _, candidate := range candidates {
			if file, ok := fileByPath[canonicalPath(candidate)]; ok {
				matched[file.DisplayPath] = struct{}{}
				found = true
			}
		}
		if !found {
			missing++
		}
		explicit = append(explicit, displayCLIPath(filepath.Clean(cwdAbsolute)))
	}
	sort.Strings(explicit)
	explicit = compactStrings(explicit)
	config := &model.FocusConfig{
		Mode:          "explicit",
		ExplicitPaths: explicit,
		ChangedPaths:  make([]string, 0),
		DeletedPaths:  make([]string, 0),
	}
	if len(changes) > 0 {
		config.Mode = "git"
		if len(values) > 0 {
			config.Mode = "git-and-explicit"
		}
		config.WorkingTreeIncluded = true
		config.UntrackedIncluded = true
		if !worktreeMode && len(changes) == 1 {
			change := changes[0]
			config.RequestedBase = change.RequestedBase
			config.BaseCommit = change.BaseCommit
			config.MergeBase = change.MergeBase
			config.HeadCommit = change.HeadCommit
			config.ChangedPaths = append(config.ChangedPaths, change.ChangedPaths...)
			config.DeletedPaths = append(config.DeletedPaths, change.DeletedPaths...)
		} else {
			config.Mode = "git-worktrees"
			if len(values) > 0 {
				config.Mode = "git-worktrees-and-explicit"
			}
			for _, change := range changes {
				root := displayCLIPath(change.Root)
				config.Worktrees = append(config.Worktrees, model.WorktreeFocusConfig{
					Root:                root,
					RequestedBase:       change.RequestedBase,
					BaseCommit:          change.BaseCommit,
					MergeBase:           change.MergeBase,
					HeadCommit:          change.HeadCommit,
					WorkingTreeIncluded: true,
					UntrackedIncluded:   true,
					ChangedPaths:        append([]string{}, change.ChangedPaths...),
					DeletedPaths:        append([]string{}, change.DeletedPaths...),
				})
				for _, path := range change.ChangedPaths {
					config.ChangedPaths = append(config.ChangedPaths, displayWorktreePath(root, path))
				}
				for _, path := range change.DeletedPaths {
					config.DeletedPaths = append(config.DeletedPaths, displayWorktreePath(root, path))
				}
			}
			sort.Slice(config.Worktrees, func(left, right int) bool {
				return config.Worktrees[left].Root < config.Worktrees[right].Root
			})
			sort.Strings(config.ChangedPaths)
			sort.Strings(config.DeletedPaths)
		}
		for _, change := range changes {
			for _, changedPath := range change.ChangedPaths {
				file, ok := fileByPath[canonicalPath(filepath.Join(change.Root, filepath.FromSlash(changedPath)))]
				if ok {
					matched[file.DisplayPath] = struct{}{}
				} else {
					missing++
				}
			}
		}
	}
	warnings := make([]model.Warning, 0)
	if missing > 0 {
		warnings = append(warnings, model.Warning{
			Kind:    "focus",
			Message: fmt.Sprintf("%d focused path(s) were excluded, unsupported, or not discovered", missing),
		})
	}
	config.DiscoveredFocusFiles = len(matched)
	return config, matched, warnings, nil
}

func canonicalPath(path string) string {
	clean := filepath.Clean(path)
	canonical, err := filepath.EvalSymlinks(clean)
	if err == nil {
		return canonical
	}
	return clean
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func expandLanguagePairs(values []string) ([]analyzer.LanguagePair, error) {
	result := make([]analyzer.LanguagePair, 0)
	for _, value := range values {
		parts := strings.Split(value, ",")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid --language-pair %q; expected left,right", value)
		}
		leftIDs, ok := language.ResolveSelector(parts[0])
		if !ok {
			return nil, fmt.Errorf("unknown language ID or family %q", strings.TrimSpace(parts[0]))
		}
		rightIDs, ok := language.ResolveSelector(parts[1])
		if !ok {
			return nil, fmt.Errorf("unknown language ID or family %q", strings.TrimSpace(parts[1]))
		}
		for _, leftID := range leftIDs {
			for _, rightID := range rightIDs {
				leftSpec, _ := language.Lookup(leftID)
				rightSpec, _ := language.Lookup(rightID)
				if leftSpec.ComparisonDomain != rightSpec.ComparisonDomain {
					return nil, fmt.Errorf(
						"incompatible comparison domains for --language-pair %s,%s: %s and %s",
						leftID,
						rightID,
						leftSpec.ComparisonDomain,
						rightSpec.ComparisonDomain,
					)
				}
				result = append(result, analyzer.LanguagePair{Left: leftID, Right: rightID})
			}
		}
	}
	return result, nil
}

func resolveComparisonDomain(value string) (string, map[string]struct{}, error) {
	registered := language.ComparisonDomains()
	known := make(map[string]struct{}, len(registered))
	for _, domain := range registered {
		known[domain] = struct{}{}
	}
	domain := strings.ToLower(strings.TrimSpace(value))
	if domain == "" {
		return "", nil, nil
	}
	if _, ok := known[domain]; !ok {
		return "", nil, fmt.Errorf(
			"unknown --comparison-domain %q; expected one of %s",
			value,
			strings.Join(registered, ", "),
		)
	}
	return domain, map[string]struct{}{domain: {}}, nil
}

func validatePairDomain(pairs []analyzer.LanguagePair, domain string) error {
	if len(pairs) == 0 || domain == "" {
		return nil
	}
	for _, pair := range pairs {
		left, _ := language.Lookup(pair.Left)
		if left.ComparisonDomain != domain {
			return fmt.Errorf(
				"--language-pair %s,%s belongs to unselected comparison domain %s",
				pair.Left,
				pair.Right,
				left.ComparisonDomain,
			)
		}
	}
	return nil
}

func runLanguages(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		if err := writeLanguagesUsage(stdout); err != nil {
			return exitError
		}
		return exitSuccess
	}
	if len(args) != 0 {
		return usageError(stderr, "languages does not accept arguments")
	}
	if _, err := fmt.Fprintln(stdout, "LANGUAGE\tFAMILY\tDOMAIN\tFRAGMENTS\tEXTENSIONS\tSHEBANGS"); err != nil {
		return exitError
	}
	for _, spec := range language.All() {
		extensions := append([]string(nil), spec.Extensions...)
		sort.Strings(extensions)
		shebangs := append([]string(nil), spec.Shebangs...)
		sort.Strings(shebangs)
		if _, err := fmt.Fprintf(
			stdout,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			spec.DisplayName,
			spec.Family,
			spec.ComparisonDomain,
			strings.Join(spec.FragmentKinds(), ", "),
			strings.Join(extensions, ", "),
			strings.Join(shebangs, ", "),
		); err != nil {
			return exitError
		}
	}
	return exitSuccess
}

func writeLanguagesUsage(writer io.Writer) error {
	_, err := fmt.Fprint(
		writer,
		"Usage: mori languages\n",
		"\nList supported parser languages, review families, comparison domains, fragment kinds, file extensions, and extensionless-script shebang interpreters.\n",
		"Select one parser for discovered .sql files with --sql-dialect generic or postgresql.\n",
	)
	return err
}

func runVersion(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 0 {
		return usageError(stderr, "version does not accept arguments")
	}
	if _, err := fmt.Fprintf(
		stdout,
		"mori %s (%s, %s, %s/%s)\n",
		buildinfo.Current().Version,
		buildinfo.DisplayRevision(buildinfo.Current()),
		buildinfo.Current().SourceDate,
		buildinfo.Current().GOOS,
		buildinfo.Current().GOARCH,
	); err != nil {
		return exitError
	}
	return exitSuccess
}

func usageError(stderr io.Writer, message string) int {
	if _, err := fmt.Fprintf(stderr, "mori: %s\n", message); err != nil {
		return exitError
	}
	return exitUsage
}

func writeRootUsage(writer io.Writer) error {
	_, err := fmt.Fprint(
		writer,
		"森 (mori) — cross-language structural similarity for source code\n",
		"\nUsage:\n",
		"  mori scan [options] [path ...]\n",
		"  mori init [--profile review|explore|sql] [--stdout | [--force] [directory]]\n",
		"  mori baseline add --baseline <path> --identity <id> [options] [path ...]\n",
		"  mori baseline remove --baseline <path> --identity <id>\n",
		"  mori baseline edit --baseline <path> --identity <id> [--note <text>] [--classification <kind>]\n",
		"  mori baseline migrate --baseline <path> --accept-profile [options] [path ...]\n",
		"  mori baseline update --baseline <path> [options] [path ...]\n",
		"  mori baseline prune --baseline <path> [options] [path ...]\n",
		"  mori languages\n",
		"  mori skill install (--project <path> | --global | --target <path>)\n",
		"  mori skill --help\n",
		"  mori version\n",
		"  mori help\n",
	)
	return err
}

func writeBaselineUsage(writer io.Writer) error {
	_, err := fmt.Fprint(
		writer,
		"Usage: mori baseline <add|remove|edit|migrate|update|prune> [options] [path ...]\n",
		"\nBaseline intentional similarity candidates for review workflows.\n",
		"Use --baseline mori-baseline.json to select the baseline file.\n",
	)
	return err
}

func displayCLIPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	cwd, err := os.Getwd()
	if err == nil {
		relative, relErr := filepath.Rel(cwd, absolute)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(relative)
		}
	}
	return filepath.ToSlash(absolute)
}

func displayOptionalPath(path string) string {
	if path == "" {
		return ""
	}
	return displayCLIPath(path)
}

type stringList []string

type errorTrackingWriter struct {
	writer io.Writer
	err    error
}

func (writer *errorTrackingWriter) Write(content []byte) (int, error) {
	if writer.err != nil {
		return 0, writer.err
	}
	count, err := writer.writer.Write(content)
	if err != nil {
		writer.err = err
	}
	return count, err
}

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}
