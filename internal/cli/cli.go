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
	"strings"

	"github.com/Cyberlane/mori/internal/analyzer"
	"github.com/Cyberlane/mori/internal/baseline"
	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/diagnostic"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/report"
	"github.com/Cyberlane/mori/internal/source"
)

const (
	exitSuccess  = 0
	exitError    = 1
	exitUsage    = 2
	exitFindings = 3
)

// Run executes one CLI invocation and returns its process exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		if err := writeRootUsage(stderr); err != nil {
			return exitError
		}
		return exitUsage
	}

	switch args[0] {
	case "scan":
		return runScan(ctx, args[1:], stdout, stderr)
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
	excludes           stringList
	languagePairs      stringList
	focusPaths         stringList
	threshold          float64
	minTokens          int
	maxGroups          int
	maxOccurrences     int
	maxPairs           int
	maxFileBytes       int64
	workers            int
	format             string
	crossLanguageOnly  bool
	failOnMatch        bool
	failOnFocusedMatch bool
	baselinePath       string
	baselineScope      string
	respectIgnore      bool
	noIgnore           bool
	requestedConfig    string
	configPath         string
	noConfig           bool
	check              bool
}

func defaultScanOptions() scanOptions {
	return scanOptions{
		threshold:      0.70,
		minTokens:      12,
		maxGroups:      100,
		maxOccurrences: 20,
		maxPairs:       5_000_000,
		maxFileBytes:   2 * 1024 * 1024,
		workers:        runtime.GOMAXPROCS(0),
		format:         "text",
		baselineScope:  string(baseline.ScopeContent),
		respectIgnore:  true,
	}
}

func (options *scanOptions) bindFlags(flags *flag.FlagSet, includeCheck bool) {
	flags.Float64Var(&options.threshold, "threshold", options.threshold, "minimum weighted Jaccard score, from 0 to 1")
	flags.IntVar(&options.minTokens, "min-tokens", options.minTokens, "minimum normalized AST tokens per fragment")
	flags.IntVar(&options.maxGroups, "max-groups", options.maxGroups, "maximum reported content-pair groups; 0 is unlimited")
	flags.IntVar(&options.maxGroups, "max-matches", options.maxGroups, "deprecated alias for --max-groups")
	flags.IntVar(&options.maxOccurrences, "max-occurrences", options.maxOccurrences, "maximum locations retained per fingerprint; 0 is unlimited")
	flags.IntVar(&options.maxPairs, "max-pairs", options.maxPairs, "comparison safety limit; 0 is unlimited")
	flags.Int64Var(&options.maxFileBytes, "max-file-bytes", options.maxFileBytes, "maximum source file size; 0 is unlimited")
	flags.IntVar(&options.workers, "workers", options.workers, "parallel parser workers")
	flags.StringVar(&options.format, "format", options.format, "output format: text or json")
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
	if includeCheck {
		flags.BoolVar(&options.check, "check", false, "report stale entries without rewriting the baseline")
	}
}

func parseScanOptions(
	command string,
	args []string,
	stderr io.Writer,
	includeCheck bool,
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
	options.bindFlags(flags, includeCheck)
	flags.Usage = func() {
		fmt.Fprintf(trackedStderr, "Usage: mori %s [options] [path ...]\n", command)
		fmt.Fprintln(trackedStderr, "\nScan function-like AST fragments for structural similarity.")
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
	if request.disabled {
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
			return options, nil
		}
		path = foundPath
	}
	settings, err := config.Load(path)
	if err != nil {
		return scanOptions{}, err
	}
	applyConfig(&options, settings, filepath.Dir(path))
	options.configPath = displayCLIPath(path)
	return options, nil
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
	if settings.FailOnMatch != nil {
		options.failOnMatch = *settings.FailOnMatch
	}
	if settings.RespectIgnore != nil {
		options.respectIgnore = *settings.RespectIgnore
	}
	options.noIgnore = !options.respectIgnore
	options.excludes = append(options.excludes, settings.Excludes...)
	options.languagePairs = append(options.languagePairs, settings.LanguagePairs...)
	if settings.Baseline != "" {
		options.baselinePath = resolveConfigPath(base, settings.Baseline)
	}
	if settings.BaselineScope != "" {
		options.baselineScope = settings.BaselineScope
	}
}

func resolveConfigPath(base string, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func validateScanOptions(options scanOptions) error {
	if math.IsNaN(options.threshold) || math.IsInf(options.threshold, 0) ||
		options.threshold <= 0 || options.threshold > 1 {
		return errors.New("--threshold must be greater than 0 and at most 1")
	}
	if options.minTokens < 1 {
		return errors.New("--min-tokens must be at least 1")
	}
	if options.maxGroups < 0 || options.maxOccurrences < 0 || options.maxPairs < 0 ||
		options.maxFileBytes < 0 {
		return errors.New("maximum values cannot be negative")
	}
	if options.workers < 1 {
		return errors.New("--workers must be at least 1")
	}
	if options.format != "text" && options.format != "json" {
		return errors.New("--format must be text or json")
	}
	if options.crossLanguageOnly && len(options.languagePairs) > 0 {
		return errors.New("--cross-language-only and --language-pair cannot be used together")
	}
	if options.failOnMatch && options.failOnFocusedMatch {
		return errors.New("--fail-on-match and --fail-on-focused-match cannot be used together")
	}
	if options.failOnFocusedMatch && len(options.focusPaths) == 0 {
		return errors.New("--fail-on-focused-match requires --focus-path or --changed-since")
	}
	if _, err := expandLanguagePairs(options.languagePairs); err != nil {
		return err
	}
	if err := baseline.ValidateScope(baseline.Scope(options.baselineScope)); err != nil {
		return err
	}
	return source.ValidatePatterns(options.excludes)
}

func runScan(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("scan", args, stderr, false)
	if !ok {
		return code
	}
	suppress, err := loadSuppression(options.baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "mori: load baseline: %v\n", err)
		return exitError
	}
	result, err := executeScan(ctx, paths, options, suppress)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
	}

	if options.format == "json" {
		err = report.JSON(stdout, result)
	} else {
		err = report.Text(stdout, result)
	}
	if err != nil {
		fmt.Fprintf(stderr, "mori: write report: %v\n", err)
		return exitError
	}
	if options.failOnMatch && result.TotalMatchGroups > 0 {
		return exitFindings
	}
	if options.failOnFocusedMatch && result.TotalFocusedMatchGroups > 0 {
		return exitFindings
	}
	return exitSuccess
}

func runBaseline(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		return usageError(stderr, "baseline requires update or prune")
	}
	switch args[0] {
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
		return usageError(stderr, "baseline command must be update or prune")
	}
}

func runBaselineUpdate(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("baseline update", args, stderr, false)
	if !ok {
		return code
	}
	if len(options.focusPaths) > 0 || options.failOnFocusedMatch {
		return usageError(stderr, "focus options cannot be used with baseline update")
	}
	if options.baselinePath == "" {
		return usageError(stderr, "--baseline is required for baseline update")
	}
	options.maxGroups = 0
	options.maxOccurrences = 0
	result, err := executeScan(ctx, paths, options, nil)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
	}
	scope := baseline.Scope(options.baselineScope)
	if err := baseline.Write(options.baselinePath, result, scope); err != nil {
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

func runBaselinePrune(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	options, paths, code, ok := parseScanOptions("baseline prune", args, stderr, true)
	if !ok {
		return code
	}
	if len(options.focusPaths) > 0 || options.failOnFocusedMatch {
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
	options.maxGroups = 0
	options.maxOccurrences = 0
	result, err := executeScan(ctx, paths, options, nil)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
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
	if err := baseline.Prune(options.baselinePath, set, result); err != nil {
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

func loadSuppression(
	path string,
) (func(string, model.Location, model.Location) bool, error) {
	if path == "" {
		return nil, nil
	}
	set, err := baseline.Load(path)
	if err != nil {
		return nil, err
	}
	return set.Match, nil
}

func executeScan(
	ctx context.Context,
	paths []string,
	options scanOptions,
	suppress func(string, model.Location, model.Location) bool,
) (model.Report, error) {
	discovered, err := source.DiscoverContext(ctx, paths, source.Options{
		Excludes:     options.excludes,
		MaxFileBytes: options.maxFileBytes,
		IgnoreFiles:  options.respectIgnore,
	})
	if err != nil {
		return model.Report{}, fmt.Errorf("discover source: %w", err)
	}
	focus, focusSet, focusWarnings, err := resolveExplicitFocus(options.focusPaths, discovered.Files)
	if err != nil {
		return model.Report{}, fmt.Errorf("resolve focus: %w", err)
	}
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
		CrossLanguageOnly: options.crossLanguageOnly,
		LanguagePairs:     pairs,
		FocusPaths:        focusSet,
		FocusActive:       focus != nil,
		Suppress:          suppress,
	})
	if err != nil {
		return result, err
	}
	result.Configuration = model.EffectiveConfig{
		ConfigPath:        options.configPath,
		IgnoreFiles:       discovered.IgnoreFiles,
		RespectIgnore:     options.respectIgnore,
		Excludes:          append([]string{}, options.excludes...),
		MinTokens:         options.minTokens,
		MaxGroups:         options.maxGroups,
		MaxOccurrences:    options.maxOccurrences,
		MaxPairs:          options.maxPairs,
		MaxFileBytes:      options.maxFileBytes,
		CrossLanguageOnly: options.crossLanguageOnly,
		LanguagePairs:     append([]string{}, options.languagePairs...),
		BaselinePath:      displayOptionalPath(options.baselinePath),
		Focus:             focus,
	}
	result.Tool = buildinfo.Current()
	result.Tool.NormalizationVersion = normalize.Version
	sort.Strings(result.Configuration.Excludes)
	sort.Strings(result.Configuration.LanguagePairs)
	return result, nil
}

func resolveExplicitFocus(values []string, files []source.File) (*model.FocusConfig, map[string]struct{}, []model.Warning, error) {
	if len(values) == 0 {
		return nil, nil, nil, nil
	}
	explicit := make([]string, 0, len(values))
	requested := make(map[string]struct{}, len(values))
	for _, value := range values {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return nil, nil, nil, err
		}
		clean := filepath.Clean(absolute)
		requested[clean] = struct{}{}
		explicit = append(explicit, displayCLIPath(clean))
	}
	sort.Strings(explicit)
	explicit = compactStrings(explicit)
	matched := make(map[string]struct{})
	for _, file := range files {
		if _, ok := requested[filepath.Clean(file.Path)]; ok {
			matched[file.DisplayPath] = struct{}{}
		}
	}
	warnings := make([]model.Warning, 0)
	if len(matched) < len(requested) {
		warnings = append(warnings, model.Warning{
			Kind:    "focus",
			Message: fmt.Sprintf("%d focused path(s) were excluded, unsupported, or not discovered", len(requested)-len(matched)),
		})
	}
	return &model.FocusConfig{
		Mode:                 "explicit",
		ExplicitPaths:        explicit,
		ChangedPaths:         make([]string, 0),
		DeletedPaths:         make([]string, 0),
		DiscoveredFocusFiles: len(matched),
	}, matched, warnings, nil
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
				result = append(result, analyzer.LanguagePair{Left: leftID, Right: rightID})
			}
		}
	}
	return result, nil
}

func runLanguages(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 0 {
		return usageError(stderr, "languages does not accept arguments")
	}
	if _, err := fmt.Fprintln(stdout, "LANGUAGE\tFAMILY\tEXTENSIONS"); err != nil {
		return exitError
	}
	for _, spec := range language.All() {
		extensions := append([]string(nil), spec.Extensions...)
		sort.Strings(extensions)
		if _, err := fmt.Fprintf(
			stdout,
			"%s\t%s\t%s\n",
			spec.DisplayName,
			spec.Family,
			strings.Join(extensions, ", "),
		); err != nil {
			return exitError
		}
	}
	return exitSuccess
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
		"  mori baseline update --baseline <path> [options] [path ...]\n",
		"  mori baseline prune --baseline <path> [options] [path ...]\n",
		"  mori languages\n",
		"  mori skill install (--project <path> | --global | --target <path>)\n",
		"  mori version\n",
		"  mori help\n",
	)
	return err
}

func writeBaselineUsage(writer io.Writer) error {
	_, err := fmt.Fprint(
		writer,
		"Usage: mori baseline <update|prune> [options] [path ...]\n",
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
