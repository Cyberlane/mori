package vcs

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// IndexEntry identifies one stage-zero Git index entry without reading its blob.
type IndexEntry struct {
	Path string
	Mode string
	OID  string
}

// IndexSnapshot records the deterministic local Git index used by --staged.
type IndexSnapshot struct {
	Root         string
	HeadCommit   string
	IndexDigest  string
	Entries      []IndexEntry
	ChangedPaths []string
	DeletedPaths []string
	options      resolverOptions
}

// ResolveIndex snapshots the local Git index without writing Git objects.
func ResolveIndex(ctx context.Context, start string) (IndexSnapshot, error) {
	return resolveIndex(ctx, start, resolverOptions{
		executable: "git", timeout: commandTimeout, maxOutput: maxOutputBytes, maxPaths: maxPaths,
	})
}

func resolveIndex(ctx context.Context, start string, options resolverOptions) (IndexSnapshot, error) {
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
	rootOutput, err := run(ctx, options, start, "rev-parse", "--show-toplevel")
	if err != nil {
		return IndexSnapshot{}, fmt.Errorf("resolve Git worktree: %w", err)
	}
	root := filepath.Clean(strings.TrimSpace(string(rootOutput)))
	if !filepath.IsAbs(root) {
		return IndexSnapshot{}, errors.New("Git returned a non-absolute worktree root")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return IndexSnapshot{}, fmt.Errorf("canonicalize Git worktree: %w", err)
	}
	indexOutput, err := run(ctx, options, root, "ls-files", "--stage", "-z", "--")
	if err != nil {
		return IndexSnapshot{}, fmt.Errorf("read Git index: %w", err)
	}
	entries, err := parseIndexEntries(indexOutput, options.maxPaths)
	if err != nil {
		return IndexSnapshot{}, err
	}
	head := ""
	headOutput, headErr := run(ctx, options, root, "rev-parse", "--verify", "HEAD^{commit}")
	if headErr == nil {
		head = strings.TrimSpace(string(headOutput))
		if !isFullSHA(head) {
			return IndexSnapshot{}, errors.New("Git returned an invalid HEAD")
		}
	}
	var changedOutput []byte
	if head != "" {
		changedOutput, err = run(ctx, options, root, "diff", "--cached", "--name-status", "-z", "--find-renames", "--no-ext-diff", head, "--")
	} else {
		changedOutput, err = run(ctx, options, root, "diff", "--cached", "--name-status", "-z", "--find-renames", "--no-ext-diff", "--root", "--")
	}
	if err != nil {
		return IndexSnapshot{}, fmt.Errorf("read staged Git changes: %w", err)
	}
	changed, deleted, err := parseNameStatus(changedOutput, options.maxPaths)
	if err != nil {
		return IndexSnapshot{}, err
	}
	hash := sha256.New()
	for _, entry := range entries {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00", entry.Mode, entry.OID, entry.Path)
	}
	return IndexSnapshot{
		Root: root, HeadCommit: head, IndexDigest: fmt.Sprintf("%x", hash.Sum(nil)), Entries: entries,
		ChangedPaths: sortedKeys(changed), DeletedPaths: sortedKeys(deleted), options: options,
	}, nil
}

func parseIndexEntries(output []byte, limit int) ([]IndexEntry, error) {
	parts := splitNUL(output)
	if len(parts) > limit {
		return nil, fmt.Errorf("Git index path count exceeded the %d path safety limit", limit)
	}
	entries := make([]IndexEntry, 0, len(parts))
	for _, record := range parts {
		metadata, path, ok := strings.Cut(record, "\t")
		if !ok {
			return nil, errors.New("Git returned malformed index metadata")
		}
		fields := strings.Fields(metadata)
		if len(fields) != 3 {
			return nil, errors.New("Git returned malformed index entry")
		}
		if fields[2] != "0" {
			return nil, fmt.Errorf("Git index contains an unmerged entry at %s", filepath.Base(path))
		}
		if err := validateRelativeGitPath(path); err != nil {
			return nil, err
		}
		if !isObjectID(fields[1]) {
			return nil, errors.New("Git returned an invalid index object ID")
		}
		entries = append(entries, IndexEntry{Path: filepath.ToSlash(path), Mode: fields[0], OID: fields[1]})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func validateRelativeGitPath(path string) error {
	if path == "" || filepath.IsAbs(path) {
		return errors.New("Git returned an unsafe index path")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("Git returned an unsafe index path")
	}
	return nil
}

func isObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := strconv.ParseUint(value[:16], 16, 64)
	if err != nil {
		return false
	}
	for _, character := range value[16:] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

// ReadIndexBlob reads one bounded blob from the snapshotted index.
func ReadIndexBlob(ctx context.Context, snapshot IndexSnapshot, entry IndexEntry, limit int64) ([]byte, int64, error) {
	sizeOutput, err := run(ctx, snapshot.options, snapshot.Root, "cat-file", "-s", entry.OID)
	if err != nil {
		return nil, 0, fmt.Errorf("inspect staged blob %s: %w", filepath.Base(entry.Path), err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
	if err != nil || size < 0 {
		return nil, 0, errors.New("Git returned an invalid staged blob size")
	}
	if limit > 0 && size > limit {
		return nil, size, nil
	}
	options := snapshot.options
	if size >= int64(options.maxOutput) {
		options.maxOutput = int(size + 1)
	}
	content, err := run(ctx, options, snapshot.Root, "cat-file", "blob", entry.OID)
	if err != nil {
		return nil, 0, fmt.Errorf("read staged blob %s: %w", filepath.Base(entry.Path), err)
	}
	if int64(len(content)) != size {
		return nil, 0, errors.New("staged blob size changed while reading")
	}
	return content, size, nil
}

// IndexEntryForPath finds an exact project-relative entry.
func IndexEntryForPath(snapshot IndexSnapshot, path string) (IndexEntry, bool) {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	index := sort.Search(len(snapshot.Entries), func(index int) bool { return snapshot.Entries[index].Path >= path })
	if index < len(snapshot.Entries) && snapshot.Entries[index].Path == path {
		return snapshot.Entries[index], true
	}
	return IndexEntry{}, false
}

// RelativeIndexPath resolves a real path inside the snapshot root.
func RelativeIndexPath(snapshot IndexSnapshot, path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(absolute)
	if !strings.HasPrefix(clean+string(filepath.Separator), snapshot.Root+string(filepath.Separator)) && clean != snapshot.Root {
		return "", errors.New("staged path is outside the Git worktree")
	}
	relative, err := filepath.Rel(snapshot.Root, clean)
	if err != nil {
		return "", err
	}
	if relative == "." {
		return ".", nil
	}
	if err := validateRelativeGitPath(relative); err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}
