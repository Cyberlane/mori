// Package cli implements the mori command-line interface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"runtime"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/analyzer"
	"github.com/Cyberlane/mori/internal/baseline"
	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
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
	excludes          stringList
	threshold         float64
	minTokens         int
	maxMatches        int
	maxPairs          int
	maxFileBytes      int64
	workers           int
	format            string
	crossLanguageOnly bool
	failOnMatch       bool
	baselinePath      string
	check             bool
}

func defaultScanOptions() scanOptions {
	return scanOptions{
		threshold:    0.70,
		minTokens:    12,
		maxMatches:   100,
		maxPairs:     5_000_000,
		maxFileBytes: 2 * 1024 * 1024,
		workers:      runtime.GOMAXPROCS(0),
		format:       "text",
	}
}

func (options *scanOptions) bindFlags(flags *flag.FlagSet, includeCheck bool) {
	flags.Float64Var(&options.threshold, "threshold", options.threshold, "minimum weighted Jaccard score, from 0 to 1")
	flags.IntVar(&options.minTokens, "min-tokens", options.minTokens, "minimum normalized AST tokens per fragment")
	flags.IntVar(&options.maxMatches, "max-matches", options.maxMatches, "maximum reported matches; 0 is unlimited")
	flags.IntVar(&options.maxPairs, "max-pairs", options.maxPairs, "comparison safety limit; 0 is unlimited")
	flags.Int64Var(&options.maxFileBytes, "max-file-bytes", options.maxFileBytes, "maximum source file size; 0 is unlimited")
	flags.IntVar(&options.workers, "workers", options.workers, "parallel parser workers")
	flags.StringVar(&options.format, "format", options.format, "output format: text or json")
	flags.BoolVar(
		&options.crossLanguageOnly,
		"cross-language-only",
		false,
		"compare fragments only when their language IDs differ",
	)
	flags.BoolVar(
		&options.failOnMatch,
		"fail-on-match",
		false,
		"exit with status 3 when one or more matches are found",
	)
	flags.StringVar(
		&options.baselinePath,
		"baseline",
		"",
		"baseline file to load or write",
	)
	flags.Var(&options.excludes, "exclude", "exclude path glob; repeat for multiple patterns")
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
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)
	options := defaultScanOptions()
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
	if err := validateScanOptions(options); err != nil {
		return scanOptions{}, nil, usageError(stderr, err.Error()), false
	}
	return options, flags.Args(), exitSuccess, true
}

func validateScanOptions(options scanOptions) error {
	if math.IsNaN(options.threshold) || math.IsInf(options.threshold, 0) ||
		options.threshold <= 0 || options.threshold > 1 {
		return errors.New("--threshold must be greater than 0 and at most 1")
	}
	if options.minTokens < 1 {
		return errors.New("--min-tokens must be at least 1")
	}
	if options.maxMatches < 0 || options.maxPairs < 0 || options.maxFileBytes < 0 {
		return errors.New("maximum values cannot be negative")
	}
	if options.workers < 1 {
		return errors.New("--workers must be at least 1")
	}
	if options.format != "text" && options.format != "json" {
		return errors.New("--format must be text or json")
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
	if options.failOnMatch && result.TotalMatches > 0 {
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
	if options.baselinePath == "" {
		return usageError(stderr, "--baseline is required for baseline update")
	}
	options.maxMatches = 0
	result, err := executeScan(ctx, paths, options, nil)
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
	}
	if err := baseline.Write(options.baselinePath, result); err != nil {
		fmt.Fprintf(stderr, "mori: write baseline: %v\n", err)
		return exitError
	}
	if _, err := fmt.Fprintf(
		stdout,
		"baseline updated: %q (%s, %s)\n",
		options.baselinePath,
		countLabel(len(result.Matches), "candidate", "candidates"),
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
	if options.baselinePath == "" {
		return usageError(stderr, "--baseline is required for baseline prune")
	}
	set, err := baseline.Load(options.baselinePath)
	if err != nil {
		fmt.Fprintf(stderr, "mori: load baseline: %v\n", err)
		return exitError
	}
	options.maxMatches = 0
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
		"baseline pruned: %q (%s removed, %s)\n",
		options.baselinePath,
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

func loadSuppression(path string) (func(string) bool, error) {
	if path == "" {
		return nil, nil
	}
	set, err := baseline.Load(path)
	if err != nil {
		return nil, err
	}
	return set.Has, nil
}

func executeScan(
	ctx context.Context,
	paths []string,
	options scanOptions,
	suppress func(string) bool,
) (model.Report, error) {
	discovered, err := source.DiscoverContext(ctx, paths, source.Options{
		Excludes:     options.excludes,
		MaxFileBytes: options.maxFileBytes,
	})
	if err != nil {
		return model.Report{}, fmt.Errorf("discover source: %w", err)
	}
	return analyzer.Analyze(ctx, discovered.Files, discovered.Warnings, analyzer.Options{
		Threshold:         options.threshold,
		MinTokens:         options.minTokens,
		MaxMatches:        options.maxMatches,
		MaxPairs:          options.maxPairs,
		Workers:           options.workers,
		CrossLanguageOnly: options.crossLanguageOnly,
		Suppress:          suppress,
	})
}

func runLanguages(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 0 {
		return usageError(stderr, "languages does not accept arguments")
	}
	if _, err := fmt.Fprintln(stdout, "LANGUAGE\tEXTENSIONS"); err != nil {
		return exitError
	}
	for _, spec := range language.All() {
		extensions := append([]string(nil), spec.Extensions...)
		sort.Strings(extensions)
		if _, err := fmt.Fprintf(
			stdout,
			"%s\t%s\n",
			spec.DisplayName,
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
		buildinfo.Version,
		buildinfo.Commit,
		buildinfo.Date,
		runtime.GOOS,
		runtime.GOARCH,
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
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("exclude pattern cannot be empty")
	}
	*values = append(*values, value)
	return nil
}
