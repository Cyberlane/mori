package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/Cyberlane/mori/internal/projectcontract"
)

// enforceProjectCompatibility is a local, network-free gate for projects that
// have opted into Mori management through a version pin or project contract.
// Projects without either marker retain the ordinary standalone CLI behavior.
func enforceProjectCompatibility(
	ctx context.Context,
	options scanOptions,
	paths []string,
	stderr io.Writer,
) int {
	roots, err := managedProjectRoots(options, paths)
	if err != nil {
		return commandError(stderr, "project compatibility gate", err)
	}
	for _, root := range roots {
		plan, err := inspectProjectUpgrade(ctx, root, "gate")
		if err != nil {
			return commandError(stderr, "project compatibility gate", err)
		}
		blocking := make([]projectUpgradeComponent, 0)
		for _, component := range plan.Components {
			if component.Classification == "required" ||
				(component.Classification == "conflict/manual" && component.Ownership == "mori-managed") {
				blocking = append(blocking, component)
			}
		}
		if len(blocking) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(stderr, "mori: project compatibility gate: %s requires Mori maintenance\n", displayCLIPath(root)); err != nil {
			return exitError
		}
		for _, component := range blocking {
			if _, err := fmt.Fprintf(stderr, "mori:   %s: %s (%s)\n", component.Name, component.Status, component.Classification); err != nil {
				return exitError
			}
		}
		if _, err := fmt.Fprintf(stderr, "mori: run 'mori project upgrade --dry-run %s' before scanning or review\n", displayCLIPath(root)); err != nil {
			return exitError
		}
		return exitUpgrade
	}
	return exitSuccess
}

func managedProjectRoots(options scanOptions, paths []string) ([]string, error) {
	starts := make([]string, 0, len(paths)+len(options.scopeRoots)+3)
	if options.stagedSnapshot != nil {
		starts = append(starts, options.stagedSnapshot.Root)
	}
	if options.configPath != "" {
		starts = append(starts, filepath.Dir(options.configPath))
	}
	if options.stagedSnapshot == nil && options.configPath == "" && len(paths) == 0 && len(options.scopeRoots) == 0 {
		if cwd, err := os.Getwd(); err == nil {
			starts = append(starts, cwd)
		} else {
			return nil, err
		}
	}
	starts = append(starts, paths...)
	starts = append(starts, options.scopeRoots...)
	roots := make(map[string]struct{})
	for _, start := range starts {
		root, found, err := discoverManagedProjectRoot(start)
		if err != nil {
			return nil, err
		}
		if found {
			roots[root] = struct{}{}
		}
	}
	result := make([]string, 0, len(roots))
	for root := range roots {
		result = append(result, root)
	}
	sort.Strings(result)
	return result, nil
}

func discoverManagedProjectRoot(start string) (string, bool, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", false, err
	}
	if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() {
		absolute = filepath.Dir(absolute)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", false, statErr
	} else if errors.Is(statErr, os.ErrNotExist) {
		absolute = filepath.Dir(absolute)
	}
	for current := filepath.Clean(absolute); ; current = filepath.Dir(current) {
		for _, name := range []string{projectcontract.FileName, ".mori-version"} {
			if _, err := os.Lstat(filepath.Join(current, name)); err == nil {
				return current, true, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", false, err
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false, nil
		}
	}
}
