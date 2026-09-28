package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/Cyberlane/mori/internal/vcs"
)

// managedOutputAuto selects Mori's rotated report directory for --output.
const managedOutputAuto = "auto"

// managedReportsKept bounds the managed report directory. Explicit --output
// paths are never counted or pruned.
const managedReportsKept = 20

// managedReportName matches only files Mori created in the managed directory;
// anything else there is left untouched.
var managedReportName = regexp.MustCompile(`^(scan|staged-check|staged-acknowledge)-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z\.json$`)

// resolveManagedReportPath returns a new managed report path under the Git
// common directory (shared by linked worktrees) and its parent directory.
func resolveManagedReportPath(ctx context.Context, label string, paths []string, now time.Time) (string, string, error) {
	start, err := managedReportStart(paths)
	if err != nil {
		return "", "", err
	}
	common, err := vcs.CommonMetadataDir(ctx, start)
	if err != nil {
		return "", "", errors.New("--output auto requires a Git repository; pass an explicit --output path")
	}
	directory := filepath.Join(common, "mori", "reports")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("create managed report directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", "", fmt.Errorf("inspect managed report directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", "", errors.New("managed report directory must be a directory, not a symlink")
	}
	name := fmt.Sprintf("%s-%s.json", label, now.UTC().Format("20060102T150405.000000000Z"))
	if !managedReportName.MatchString(name) {
		return "", "", fmt.Errorf("invalid managed report name %q", name)
	}
	return filepath.Join(directory, name), directory, nil
}

func managedReportStart(paths []string) (string, error) {
	if len(paths) == 0 || paths[0] == "-" {
		return filepath.Abs(".")
	}
	start, err := filepath.Abs(paths[0])
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		start = filepath.Dir(start)
	}
	return start, nil
}

func managedReportLabel(mode string) string {
	switch mode {
	case "staged-check", "staged-acknowledge":
		return mode
	default:
		return "scan"
	}
}

// pruneManagedReports keeps the newest kept Mori-named regular files. Names
// sort chronologically, so no filesystem timestamps are consulted.
func pruneManagedReports(directory string, kept int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && managedReportName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	if len(names) <= kept {
		return nil
	}
	// Order by timestamp, not by command label.
	sort.Slice(names, func(i, j int) bool {
		left, right := managedReportTimestamp(names[i]), managedReportTimestamp(names[j])
		if left != right {
			return left < right
		}
		return names[i] < names[j]
	})
	var failures []error
	for _, name := range names[:len(names)-kept] {
		if err := os.Remove(filepath.Join(directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func managedReportTimestamp(name string) string {
	match := managedReportName.FindStringSubmatch(name)
	if match == nil {
		return name
	}
	return name[len(match[1])+1:]
}
