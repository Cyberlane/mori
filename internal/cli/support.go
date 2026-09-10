package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Cyberlane/mori/internal/support"
)

func runSupport(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageError(stderr, "support requires bundle or inspect")
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if len(args) > 1 {
			return usageError(stderr, "support help accepts no arguments")
		}
		if err := writeSupportUsage(stdout); err != nil {
			return exitError
		}
		return exitSuccess
	}
	action := args[0]
	if action != "bundle" && action != "inspect" {
		return usageError(stderr, "support requires bundle or inspect")
	}
	flags := flag.NewFlagSet("support "+action, flag.ContinueOnError)
	tracked := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(tracked)
	var sessionPath, outputPath string
	var findingLabels stringList
	if action == "bundle" {
		flags.Var(&findingLabels, "finding", "reviewed shortlist rank and label as RANK:useful|intentional|false-positive|uncertain; repeat up to 25")
		flags.StringVar(&sessionPath, "session", "", "validated local support session JSON")
		flags.StringVar(&outputPath, "output", "", "new local ZIP file; existing paths are never overwritten")
	}
	flags.Usage = func() {
		_ = writeSupportUsage(tracked)
		if action == "bundle" {
			flags.PrintDefaults()
		}
	}
	if err := flags.Parse(args[1:]); err != nil {
		if tracked.err != nil {
			return exitError
		}
		if errors.Is(err, flag.ErrHelp) {
			return exitSuccess
		}
		return exitUsage
	}
	if err := validateOptionPlacement(args[1:], flags.Args(), flags); err != nil {
		return usageError(stderr, err.Error())
	}
	if action == "inspect" {
		if flags.NArg() != 1 {
			return usageError(stderr, "support inspect requires one session JSON or support ZIP path")
		}
		session, err := support.Inspect(flags.Arg(0))
		if err != nil {
			return commandError(stderr, "inspect support evidence", err)
		}
		content, err := support.Marshal(session)
		if err != nil {
			return commandError(stderr, "validate support evidence", err)
		}
		if _, err := stdout.Write(content); err != nil {
			return exitError
		}
		return exitSuccess
	}
	if flags.NArg() != 0 || sessionPath == "" || outputPath == "" {
		return usageError(stderr, "support bundle requires --session and --output, with no positional paths")
	}
	session, err := support.ReadSession(sessionPath)
	if err != nil {
		return commandError(stderr, "read support session", err)
	}
	if len(findingLabels) > 0 && !session.ReportAvailable {
		return usageError(stderr, "--finding requires a session with report evidence")
	}
	for _, label := range findingLabels {
		rankText, classification, found := strings.Cut(label, ":")
		rank, parseErr := strconv.Atoi(rankText)
		if !found || parseErr != nil || rank < 1 || rank > 1_000_000 {
			return usageError(stderr, "--finding requires a rank from 1 to 1000000 and a classification as RANK:LABEL")
		}
		switch classification {
		case "useful", "intentional", "false-positive", "uncertain":
		default:
			return usageError(stderr, "--finding classification must be useful, intentional, false-positive, or uncertain")
		}
		for _, previous := range session.ReviewedFindings {
			if previous.Rank == rank {
				return usageError(stderr, "--finding ranks must be unique, including existing session annotations")
			}
		}
		session.ReviewedFindings = append(session.ReviewedFindings, support.ReviewedFinding{Rank: rank, Classification: classification})
		if len(session.ReviewedFindings) > 25 {
			return usageError(stderr, "support evidence allows at most 25 reviewed findings")
		}
	}
	// Marshal validates the exact allowlisted payload before any output mutation.
	preview, err := support.Marshal(session)
	if err != nil {
		return commandError(stderr, "validate support session", err)
	}
	if err := support.WriteBundle(outputPath, session); err != nil {
		return commandError(stderr, "write support bundle", err)
	}
	if _, err := stdout.Write(preview); err != nil {
		return exitError
	}
	if _, err := fmt.Fprintln(stderr, "Support bundle created locally. Standard output previews its session JSON. Nothing was uploaded."); err != nil {
		return exitError
	}
	return exitSuccess
}

func writeSupportUsage(writer io.Writer) error {
	_, err := fmt.Fprint(writer, "Usage: mori support bundle --session <session.json> --output <new.zip>\n       mori support inspect [--] <session.json|support.zip>\n\nInspect prints the validated, shareable session JSON. Bundles contain only\nsession.json and a fixed README.txt. No source attachments or network submission.\n")
	return err
}
