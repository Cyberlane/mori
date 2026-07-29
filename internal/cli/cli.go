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
	"github.com/Cyberlane/mori/internal/buildinfo"
	"github.com/Cyberlane/mori/internal/language"
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
	case "languages":
		return runLanguages(args[1:], stdout, stderr)
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

func runScan(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)

	var excludes stringList
	threshold := flags.Float64("threshold", 0.70, "minimum weighted Jaccard score, from 0 to 1")
	minTokens := flags.Int("min-tokens", 12, "minimum normalized AST tokens per fragment")
	maxMatches := flags.Int("max-matches", 100, "maximum reported matches; 0 is unlimited")
	maxPairs := flags.Int("max-pairs", 5_000_000, "comparison safety limit; 0 is unlimited")
	maxFileBytes := flags.Int64("max-file-bytes", 2*1024*1024, "maximum source file size; 0 is unlimited")
	workers := flags.Int("workers", runtime.GOMAXPROCS(0), "parallel parser workers")
	format := flags.String("format", "text", "output format: text or json")
	crossLanguageOnly := flags.Bool(
		"cross-language-only",
		false,
		"compare fragments only when their language IDs differ",
	)
	failOnMatch := flags.Bool(
		"fail-on-match",
		false,
		"exit with status 3 when one or more matches are found",
	)
	flags.Var(&excludes, "exclude", "exclude path glob; repeat for multiple patterns")
	flags.Usage = func() {
		fmt.Fprintln(trackedStderr, "Usage: mori scan [options] [path ...]")
		fmt.Fprintln(trackedStderr, "\nScan function-like AST fragments for structural similarity.")
		fmt.Fprintln(trackedStderr, "Paths default to the current directory.")
		fmt.Fprintln(trackedStderr, "\nOptions:")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if trackedStderr.err != nil {
				return exitError
			}
			return exitSuccess
		}
		if trackedStderr.err != nil {
			return exitError
		}
		return exitUsage
	}
	if math.IsNaN(*threshold) || math.IsInf(*threshold, 0) ||
		*threshold <= 0 || *threshold > 1 {
		return usageError(stderr, "--threshold must be greater than 0 and at most 1")
	}
	if *minTokens < 1 {
		return usageError(stderr, "--min-tokens must be at least 1")
	}
	if *maxMatches < 0 || *maxPairs < 0 || *maxFileBytes < 0 {
		return usageError(stderr, "maximum values cannot be negative")
	}
	if *workers < 1 {
		return usageError(stderr, "--workers must be at least 1")
	}
	if *format != "text" && *format != "json" {
		return usageError(stderr, "--format must be text or json")
	}
	if err := source.ValidatePatterns(excludes); err != nil {
		return usageError(stderr, err.Error())
	}

	discovered, err := source.DiscoverContext(ctx, flags.Args(), source.Options{
		Excludes:     excludes,
		MaxFileBytes: *maxFileBytes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "mori: discover source: %v\n", err)
		return exitError
	}
	result, err := analyzer.Analyze(ctx, discovered.Files, discovered.Warnings, analyzer.Options{
		Threshold:         *threshold,
		MinTokens:         *minTokens,
		MaxMatches:        *maxMatches,
		MaxPairs:          *maxPairs,
		Workers:           *workers,
		CrossLanguageOnly: *crossLanguageOnly,
	})
	if err != nil {
		fmt.Fprintf(stderr, "mori: %v\n", err)
		return exitError
	}

	if *format == "json" {
		err = report.JSON(stdout, result)
	} else {
		err = report.Text(stdout, result)
	}
	if err != nil {
		fmt.Fprintf(stderr, "mori: write report: %v\n", err)
		return exitError
	}
	if *failOnMatch && result.TotalMatches > 0 {
		return exitFindings
	}
	return exitSuccess
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
		"  mori languages\n",
		"  mori version\n",
		"  mori help\n",
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
