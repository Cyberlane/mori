// Package vcs resolves bounded, local version-control state for review focus.
package vcs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/pathutil"
)

const (
	commandTimeout = 15 * time.Second
	maxOutputBytes = 16 * 1024 * 1024
	maxPaths       = 100_000
)

var errOutputLimit = errors.New("git output exceeded the 16 MiB safety limit")

// Changes is a deterministic snapshot of paths changed from a merge base
// through the current working tree.
type Changes struct {
	Root          string
	RequestedBase string
	BaseCommit    string
	MergeBase     string
	HeadCommit    string
	ChangedPaths  []string
	DeletedPaths  []string
	// WholeFileFocusPaths contains untracked paths that have no diff hunk and
	// therefore require path-wide focus.
	WholeFileFocusPaths []string
	// ChangedLineIntervals contains added/nearest-surviving new-file lines by
	// repository-relative path. A deletion-only hunk is represented by the
	// new-file insertion point and the immediately following line (with line 1
	// as the minimum), conservatively focusing the nearest surviving function.
	ChangedLineIntervals map[string][]model.LineInterval
}

type resolverOptions struct {
	executable string
	timeout    time.Duration
	maxOutput  int
	maxPaths   int
}

// ResolveChanged resolves revision and local worktree changes without network
// access. filePaths must be absolute discovered source paths.
func ResolveChanged(ctx context.Context, filePaths []string, revision string) (Changes, error) {
	return resolveChanged(ctx, filePaths, revision, resolverOptions{
		executable: "git",
		timeout:    commandTimeout,
		maxOutput:  maxOutputBytes,
		maxPaths:   maxPaths,
	})
}

// ResolveChangedAtRoot resolves revision and local changes for one explicitly
// named Git worktree without network access. root must be that worktree's
// canonical top level, not merely a directory contained by it.
func ResolveChangedAtRoot(ctx context.Context, root string, revision string) (Changes, error) {
	return resolveChangedAtRoot(ctx, root, revision, resolverOptions{
		executable: "git",
		timeout:    commandTimeout,
		maxOutput:  maxOutputBytes,
		maxPaths:   maxPaths,
	})
}

func resolveChanged(
	ctx context.Context,
	filePaths []string,
	revision string,
	options resolverOptions,
) (Changes, error) {
	if strings.TrimSpace(revision) == "" {
		return Changes{}, errors.New("Git comparison revision cannot be empty")
	}
	if options.executable == "" {
		options.executable = "git"
	}
	if options.timeout <= 0 {
		options.timeout = commandTimeout
	}
	if options.maxOutput <= 0 {
		options.maxOutput = maxOutputBytes
	}
	if options.maxPaths <= 0 {
		options.maxPaths = maxPaths
	}

	start, err := startDirectory(filePaths)
	if err != nil {
		return Changes{}, err
	}
	rootOutput, err := run(ctx, options, start, "rev-parse", "--show-toplevel")
	if err != nil {
		return Changes{}, fmt.Errorf("resolve Git worktree: %w", err)
	}
	root := filepath.Clean(strings.TrimSpace(string(rootOutput)))
	if !filepath.IsAbs(root) {
		return Changes{}, errors.New("Git returned a non-absolute worktree root")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Changes{}, fmt.Errorf("canonicalize Git worktree: %w", err)
	}
	for _, filePath := range filePaths {
		canonicalPath, canonicalErr := filepath.EvalSymlinks(filepath.Clean(filePath))
		if canonicalErr != nil {
			return Changes{}, fmt.Errorf("canonicalize discovered path: %w", canonicalErr)
		}
		if !pathutil.Within(root, canonicalPath) {
			return Changes{}, fmt.Errorf("scan spans multiple Git worktrees: %s", filepath.Base(filePath))
		}
		boundary, boundaryErr := nestedGitBoundary(root, filepath.Dir(canonicalPath))
		if boundaryErr != nil {
			return Changes{}, boundaryErr
		}
		if boundary != "" {
			relative, relErr := filepath.Rel(root, boundary)
			if relErr != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return Changes{}, errors.New("scan spans multiple Git worktrees")
			}
			return Changes{}, fmt.Errorf(
				"scan spans multiple Git worktrees at %s; exclude and scan that worktree separately or use --focus-path",
				filepath.ToSlash(relative),
			)
		}
	}
	return resolveChangedInCanonicalRoot(ctx, root, revision, "--changed-since", options)
}

func resolveChangedAtRoot(
	ctx context.Context,
	root string,
	revision string,
	options resolverOptions,
) (Changes, error) {
	if strings.TrimSpace(revision) == "" {
		return Changes{}, errors.New("Git comparison revision cannot be empty")
	}
	if options.executable == "" {
		options.executable = "git"
	}
	if options.timeout <= 0 {
		options.timeout = commandTimeout
	}
	if options.maxOutput <= 0 {
		options.maxOutput = maxOutputBytes
	}
	if options.maxPaths <= 0 {
		options.maxPaths = maxPaths
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Changes{}, fmt.Errorf("resolve explicit Git worktree: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return Changes{}, fmt.Errorf("canonicalize explicit Git worktree: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return Changes{}, fmt.Errorf("inspect explicit Git worktree: %w", err)
	}
	if !info.IsDir() {
		return Changes{}, errors.New("explicit Git worktree is not a directory")
	}
	rootOutput, err := run(ctx, options, canonical, "rev-parse", "--show-toplevel")
	if err != nil {
		return Changes{}, fmt.Errorf("resolve explicit Git worktree: %w", err)
	}
	resolved := filepath.Clean(strings.TrimSpace(string(rootOutput)))
	if !filepath.IsAbs(resolved) {
		return Changes{}, errors.New("Git returned a non-absolute worktree root")
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return Changes{}, fmt.Errorf("canonicalize Git worktree: %w", err)
	}
	if resolved != canonical {
		return Changes{}, fmt.Errorf("explicit Git worktree resolves to parent worktree %s", filepath.Base(resolved))
	}
	return resolveChangedInCanonicalRoot(ctx, canonical, revision, "--changed-worktree", options)
}

func resolveChangedInCanonicalRoot(
	ctx context.Context,
	root string,
	revision string,
	option string,
	options resolverOptions,
) (Changes, error) {

	baseOutput, err := run(
		ctx,
		options,
		root,
		"rev-parse",
		"--verify",
		"--end-of-options",
		revision+"^{commit}",
	)
	if err != nil {
		return Changes{}, fmt.Errorf("resolve %s revision %q: %w", option, revision, err)
	}
	headOutput, err := run(ctx, options, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return Changes{}, fmt.Errorf("resolve Git HEAD: %w", err)
	}
	base := strings.TrimSpace(string(baseOutput))
	head := strings.TrimSpace(string(headOutput))
	mergeOutput, err := run(ctx, options, root, "merge-base", base, head)
	if err != nil {
		return Changes{}, fmt.Errorf("resolve Git merge base: %w", err)
	}
	mergeBase := strings.TrimSpace(string(mergeOutput))
	for label, value := range map[string]string{"base commit": base, "HEAD": head, "merge base": mergeBase} {
		if !isFullSHA(value) {
			return Changes{}, fmt.Errorf("Git returned an invalid %s", label)
		}
	}

	tracked, err := run(
		ctx,
		options,
		root,
		"diff",
		"--name-status",
		"-z",
		"--find-renames",
		"--no-ext-diff",
		mergeBase,
		"--",
	)
	if err != nil {
		return Changes{}, fmt.Errorf("read tracked Git changes: %w", err)
	}
	changed, deleted, err := parseNameStatus(tracked, options.maxPaths)
	if err != nil {
		return Changes{}, err
	}
	patch, err := run(ctx, options, root, "diff", "--unified=0", "--no-ext-diff", "--no-color", "--no-renames", mergeBase, "--")
	if err != nil {
		return Changes{}, fmt.Errorf("read tracked Git line changes: %w", err)
	}
	intervals, err := parseUnifiedDiffIntervals(patch, options.maxPaths)
	if err != nil {
		return Changes{}, err
	}
	untracked, err := run(ctx, options, root, "ls-files", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return Changes{}, fmt.Errorf("read untracked Git files: %w", err)
	}
	wholeFileFocus := make(map[string]struct{})
	for _, path := range splitNUL(untracked) {
		if err := addSafePath(changed, path); err != nil {
			return Changes{}, err
		}
		if err := addSafePath(wholeFileFocus, path); err != nil {
			return Changes{}, err
		}
		if len(changed)+len(deleted) > options.maxPaths {
			return Changes{}, fmt.Errorf("Git changed path count exceeded the %d path safety limit", options.maxPaths)
		}
	}

	return Changes{
		Root:                 root,
		RequestedBase:        revision,
		BaseCommit:           base,
		MergeBase:            mergeBase,
		HeadCommit:           head,
		ChangedPaths:         sortedKeys(changed),
		DeletedPaths:         sortedKeys(deleted),
		WholeFileFocusPaths:  sortedKeys(wholeFileFocus),
		ChangedLineIntervals: intervals,
	}, nil
}

func startDirectory(filePaths []string) (string, error) {
	if len(filePaths) > 0 {
		return filepath.Dir(filePaths[0]), nil
	}
	return filepath.Abs(".")
}

func run(
	parent context.Context,
	options resolverOptions,
	directory string,
	args ...string,
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, options.timeout)
	defer cancel()
	stdout := &limitedBuffer{limit: options.maxOutput}
	stderr := &limitedBuffer{limit: 64 * 1024}
	command := exec.CommandContext(ctx, options.executable, append([]string{"-C", directory}, args...)...)
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if errors.Is(stdout.err, errOutputLimit) || errors.Is(stderr.err, errOutputLimit) {
		return nil, errOutputLimit
	}
	if err != nil {
		message := strings.TrimSpace(stderr.buffer.String())
		if message == "" {
			return nil, err
		}
		return nil, errors.New(message)
	}
	return stdout.buffer.Bytes(), nil
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
	err    error
}

func (buffer *limitedBuffer) Write(content []byte) (int, error) {
	if buffer.err != nil {
		return 0, buffer.err
	}
	remaining := buffer.limit - buffer.buffer.Len()
	if len(content) > remaining {
		if remaining > 0 {
			_, _ = buffer.buffer.Write(content[:remaining])
		}
		buffer.err = errOutputLimit
		return 0, buffer.err
	}
	return buffer.buffer.Write(content)
}

func parseNameStatus(content []byte, pathLimit int) (map[string]struct{}, map[string]struct{}, error) {
	fields := splitNUL(content)
	changed := make(map[string]struct{})
	deleted := make(map[string]struct{})
	for index := 0; index < len(fields); {
		status := fields[index]
		index++
		if status == "" {
			continue
		}
		paths := 1
		if status[0] == 'R' || status[0] == 'C' {
			paths = 2
		}
		if index+paths > len(fields) {
			return nil, nil, errors.New("Git returned malformed name-status output")
		}
		selected := fields[index]
		if paths == 2 {
			selected = fields[index+1]
		}
		index += paths
		if status[0] == 'D' {
			if err := addSafePath(deleted, selected); err != nil {
				return nil, nil, err
			}
		} else if err := addSafePath(changed, selected); err != nil {
			return nil, nil, err
		}
		if len(changed)+len(deleted) > pathLimit {
			return nil, nil, fmt.Errorf("Git changed path count exceeded the %d path safety limit", pathLimit)
		}
	}
	return changed, deleted, nil
}

var unifiedHunkPattern = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

// parseUnifiedDiffIntervals extracts inclusive new-file line ranges from a
// zero-context unified diff. Added lines are exact. For deletion-only hunks,
// Git reports no new lines, so the insertion point and following line are
// retained as deterministic nearest-surviving focus evidence.
func parseUnifiedDiffIntervals(content []byte, pathLimit int) (map[string][]model.LineInterval, error) {
	result := make(map[string][]model.LineInterval)
	currentPath := ""
	seenOldHeader := false
	inHunk := false
	for _, raw := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(raw, "diff --git ") {
			currentPath = ""
			seenOldHeader = false
			inHunk = false
			continue
		}
		// Header-looking lines are only meaningful before the first hunk. Once
		// in a hunk, all +++/--- lines are source content, not patch headers.
		if !inHunk && strings.HasPrefix(raw, "--- ") {
			seenOldHeader = true
			continue
		}
		if !inHunk && seenOldHeader && strings.HasPrefix(raw, "+++ ") {
			path, err := parsePatchPath(strings.TrimSuffix(strings.TrimPrefix(raw, "+++ "), "\r"), "b/")
			if err != nil {
				return nil, err
			}
			if path == "/dev/null" {
				currentPath = ""
			} else {
				validated := make(map[string]struct{}, 1)
				if err := addSafePath(validated, path); err != nil {
					return nil, err
				}
				currentPath = path
			}
			continue
		}
		matches := unifiedHunkPattern.FindStringSubmatch(raw)
		if matches == nil {
			continue
		}
		inHunk = true
		if currentPath == "" {
			continue
		}
		start, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, errors.New("Git returned malformed unified diff hunk")
		}
		count := 1
		if matches[2] != "" {
			count, err = strconv.Atoi(matches[2])
			if err != nil {
				return nil, errors.New("Git returned malformed unified diff hunk")
			}
		}
		if count == 0 {
			if start < 1 {
				start = 1
			}
			end := start + 1
			result[currentPath] = append(result[currentPath], model.LineInterval{StartLine: start, EndLine: end})
			continue
		}
		result[currentPath] = append(result[currentPath], model.LineInterval{StartLine: start, EndLine: start + count - 1})
	}
	if len(result) > pathLimit {
		return nil, fmt.Errorf("Git changed path count exceeded the %d path safety limit", pathLimit)
	}
	for path, intervals := range result {
		sort.Slice(intervals, func(i, j int) bool {
			if intervals[i].StartLine != intervals[j].StartLine {
				return intervals[i].StartLine < intervals[j].StartLine
			}
			return intervals[i].EndLine < intervals[j].EndLine
		})
		merged := intervals[:0]
		for _, interval := range intervals {
			if len(merged) > 0 && interval.StartLine <= merged[len(merged)-1].EndLine+1 {
				if interval.EndLine > merged[len(merged)-1].EndLine {
					merged[len(merged)-1].EndLine = interval.EndLine
				}
				continue
			}
			merged = append(merged, interval)
		}
		result[path] = merged
	}
	return result, nil
}

func parsePatchPath(value string, prefix string) (string, error) {
	if strings.HasPrefix(value, `"`) {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("Git returned malformed patch path")
		}
		value = decoded
	}
	if strings.HasPrefix(value, prefix) {
		value = strings.TrimPrefix(value, prefix)
	}
	return filepath.ToSlash(value), nil
}

func splitNUL(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	parts := bytes.Split(content, []byte{0})
	if len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, string(part))
	}
	return result
}

func addSafePath(paths map[string]struct{}, path string) error {
	if path == "" || strings.ContainsRune(path, 0) || filepath.IsAbs(path) {
		return errors.New("Git returned an unsafe changed path")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("Git returned a changed path outside the worktree")
	}
	paths[clean] = struct{}{}
	return nil
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func isFullSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func nestedGitBoundary(root string, directory string) (string, error) {
	for current := filepath.Clean(directory); current != root; current = filepath.Dir(current) {
		if !pathutil.Within(root, current) {
			return "", errors.New("discovered path is outside the Git worktree")
		}
		_, err := os.Lstat(filepath.Join(current, ".git"))
		if err == nil {
			return current, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect nested Git worktree: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", nil
}
