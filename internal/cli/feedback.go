package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Cyberlane/mori/internal/feedback"
	"github.com/Cyberlane/mori/internal/model"
)

func feedbackCI() bool {
	for _, key := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "TF_BUILD", "JENKINS_URL", "TEAMCITY_VERSION"} {
		value := strings.ToLower(os.Getenv(key))
		if value != "" && value != "false" && value != "0" {
			return true
		}
	}
	return false
}

func localFeedbackStore(root string) (*feedback.Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return feedback.Open(base, root)
}

func runFeedback(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageError(stderr, "feedback requires enable, status, disable, clear, classify, export, or summarize")
	}
	action := args[0]
	if action == "summarize" {
		return runFeedbackSummary(args[1:], stdout, stderr)
	}
	switch action {
	case "enable", "status", "disable", "clear", "classify", "export":
	default:
		return usageError(stderr, "unknown feedback action")
	}
	flags := flag.NewFlagSet("feedback "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "project directory whose local feedback consent to manage")
	var ci bool
	var classification, effort string
	var rank int
	if action == "classify" {
		flags.StringVar(&classification, "classification", "", "one reviewed finding: useful, intentional, false-positive, uncertain")
		flags.StringVar(&effort, "effort", "", "optional self-reported total review effort: under-1m, 1-5m, 5-15m, 15m+")
		flags.IntVar(&rank, "rank", 0, "optional positive reviewed shortlist rank; stored as a coarse bucket")
	}
	if action == "enable" {
		flags.BoolVar(&ci, "ci", false, "also consent to collection in CI for this user and project")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return exitSuccess
		}
		return exitUsage
	}
	if flags.NArg() != 0 {
		return usageError(stderr, "feedback accepts no positional arguments; use --root")
	}
	store, err := localFeedbackStore(*root)
	if err != nil {
		fmt.Fprintln(stderr, "mori: feedback storage unavailable")
		return exitError
	}
	switch action {
	case "enable":
		err = store.Enable(ci)
	case "disable":
		err = store.Disable()
	case "clear":
		err = store.Clear()
	case "export":
		err = store.Export(stdout)
	case "classify":
		err = store.ClassifyLatest(classification, effort, rank, feedbackCI())
	}
	if err != nil {
		fmt.Fprintln(stderr, "mori: feedback operation failed:", err)
		return exitError
	}
	if action == "export" {
		return exitSuccess
	}
	state, err := store.Read()
	if err != nil {
		fmt.Fprintln(stderr, "mori: cannot read feedback status")
		return exitError
	}
	_, err = fmt.Fprintf(stdout, "Local feedback: enabled=%t ci_enabled=%t retained_samples=%d limit=%d\nNo network submission. Export previews the exact shareable bundle. Clear deletes samples and keeps consent.\n", state.Enabled, state.CI, len(state.Samples), feedback.MaxRecords)
	if err != nil {
		return exitError
	}
	return exitSuccess
}

// captureLocalFeedback is deliberately best effort: consent, storage or lock
// failures cannot change scan output, exit policy, or deterministic receipts.
func captureLocalFeedback(root string, report model.Report, elapsed time.Duration, outcome string, cache ...string) {
	store, err := localFeedbackStore(root)
	if err != nil {
		return
	}
	_ = store.Capture(feedback.Measure(report, elapsed, outcome, cache...), feedbackCI())
}

// Summary accepts explicit exports and never opens project consent storage.
func runFeedbackSummary(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("feedback summarize", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: mori feedback summarize EXPORT.json [EXPORT.json ...] (1 to 64 files; offline)")
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitSuccess
		}
		return exitUsage
	}
	if flags.NArg() < 1 || flags.NArg() > feedback.MaxBundles {
		return usageError(stderr, "feedback summarize requires 1 to 64 export files")
	}
	bundles := make([]feedback.Bundle, 0, flags.NArg())
	for _, path := range flags.Args() {
		pathInfo, pathErr := os.Lstat(path)
		if pathErr != nil || !pathInfo.Mode().IsRegular() {
			fmt.Fprintln(stderr, "mori: feedback export must be a regular file")
			return exitError
		}
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(stderr, "mori: cannot open feedback export")
			return exitError
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			_ = file.Close()
			fmt.Fprintln(stderr, "mori: feedback export must be a regular file")
			return exitError
		}
		bundle, readErr := feedback.ReadBundle(file)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			fmt.Fprintln(stderr, "mori: invalid feedback export")
			return exitError
		}
		bundles = append(bundles, bundle)
	}
	if err := feedback.Summarize(stdout, bundles); err != nil {
		fmt.Fprintln(stderr, "mori: feedback summary failed:", err)
		return exitError
	}
	return exitSuccess
}
