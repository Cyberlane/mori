package source

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
)

// SnapshotEntry is one immutable file-like entry from a repository snapshot.
type SnapshotEntry struct {
	Path    string
	Mode    string
	Size    int64
	Content []byte
}

type snapshotNode struct {
	directories map[string]*snapshotNode
	files       map[string]SnapshotEntry
}

// DiscoverSnapshot applies normal discovery policy to an immutable repository view.
func DiscoverSnapshot(
	ctx context.Context,
	root string,
	requested []string,
	entries []SnapshotEntry,
	options Options,
) (Result, error) {
	root = filepath.Clean(root)
	if len(requested) == 0 {
		requested = []string{"."}
	}
	cwd, err := filepath.Abs(".")
	if err != nil {
		cwd = root
	}
	requested, explicitFiles, err := normalizeSnapshotRoots(root, cwd, requested, entries)
	if err != nil {
		return Result{}, err
	}
	tree := &snapshotNode{directories: map[string]*snapshotNode{}, files: map[string]SnapshotEntry{}}
	for _, entry := range entries {
		addSnapshotEntry(tree, entry)
	}
	result := Result{
		Files: make([]File, 0), Warnings: make([]model.Warning, 0),
		Excluded: make([]model.FileCoverage, 0), Unsupported: make([]model.UnsupportedExtension, 0),
	}
	state := snapshotDiscoveryState{
		root: root, cwd: cwd, requested: requested, explicitFiles: explicitFiles, options: options,
		result: &result, seen: map[string]struct{}{}, unsupportedSeen: map[string]struct{}{},
		unsupportedCounts: map[string]int{}, ignores: newIgnoreMatcher(cwd),
	}
	if err := state.walk(ctx, tree, "."); err != nil {
		return Result{}, err
	}
	result.IgnoreFiles = state.ignores.paths()
	result.IgnoreEvidence = state.ignores.evidence()
	for extension, count := range state.unsupportedCounts {
		result.Unsupported = append(result.Unsupported, model.UnsupportedExtension{Extension: extension, FileCount: count})
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].DisplayPath < result.Files[j].DisplayPath })
	sort.Slice(result.Warnings, func(i, j int) bool {
		if result.Warnings[i].Path == result.Warnings[j].Path {
			return result.Warnings[i].Message < result.Warnings[j].Message
		}
		return result.Warnings[i].Path < result.Warnings[j].Path
	})
	sort.Slice(result.Excluded, func(i, j int) bool { return result.Excluded[i].Path < result.Excluded[j].Path })
	sort.Slice(result.Unsupported, func(i, j int) bool { return result.Unsupported[i].Extension < result.Unsupported[j].Extension })
	return result, nil
}

type snapshotDiscoveryState struct {
	root              string
	cwd               string
	requested         []string
	explicitFiles     map[string]struct{}
	options           Options
	result            *Result
	seen              map[string]struct{}
	unsupportedSeen   map[string]struct{}
	unsupportedCounts map[string]int
	ignores           *ignoreMatcher
}

func (state *snapshotDiscoveryState) walk(ctx context.Context, node *snapshotNode, relative string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if relative != "." && !snapshotDirectoryRelevant(relative, state.requested) {
		return nil
	}
	absolute := state.root
	if relative != "." {
		absolute = filepath.Join(state.root, filepath.FromSlash(relative))
		if !snapshotDirectoryRequired(relative, state.requested) {
			name := filepath.Base(absolute)
			if _, excluded := defaultExcludedDirectories[strings.ToLower(name)]; excluded {
				return nil
			}
			if matchesAny(relative, displayPath(state.cwd, absolute), state.options.Excludes) {
				return nil
			}
			if state.options.IgnoreFiles && state.ignores.ignored(absolute, true) && !state.ignores.mayReinclude(absolute) {
				return nil
			}
		}
	}
	if state.options.IgnoreFiles {
		for _, name := range []string{".gitignore", ".moriignore"} {
			entry, ok := node.files[name]
			if !ok || entry.Mode == "120000" || entry.Mode == "160000" {
				continue
			}
			path := filepath.Join(absolute, name)
			if entry.Size > maxIgnoreBytes || len(entry.Content) > maxIgnoreBytes {
				return fmt.Errorf("ignore file %s exceeds %d bytes", displayPath(state.cwd, path), maxIgnoreBytes)
			}
			if _, err := state.ignores.loadContent(path, absolute, entry.Content); err != nil {
				return err
			}
		}
	}
	fileNames := make([]string, 0, len(node.files))
	for name := range node.files {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)
	for _, name := range fileNames {
		entry := node.files[name]
		pathRelative := entry.Path
		if !snapshotFileSelected(pathRelative, state.requested) {
			continue
		}
		absolutePath := filepath.Join(state.root, filepath.FromSlash(pathRelative))
		_, explicit := state.explicitFiles[pathRelative]
		if !explicit {
			if matchesAny(pathRelative, displayPath(state.cwd, absolutePath), state.options.Excludes) {
				continue
			}
			if state.options.IgnoreFiles && state.ignores.ignored(absolutePath, false) {
				continue
			}
		}
		state.addFile(entry, absolutePath, explicit)
	}
	directoryNames := make([]string, 0, len(node.directories))
	for name := range node.directories {
		directoryNames = append(directoryNames, name)
	}
	sort.Strings(directoryNames)
	for _, name := range directoryNames {
		next := name
		if relative != "." {
			next = relative + "/" + name
		}
		if err := state.walk(ctx, node.directories[name], next); err != nil {
			return err
		}
	}
	return nil
}

func (state *snapshotDiscoveryState) addFile(entry SnapshotEntry, path string, explicit bool) {
	if entry.Mode == "120000" {
		if explicit {
			state.result.Warnings = append(state.result.Warnings, model.Warning{Path: displayPath(state.cwd, path), Message: "symbolic links are not followed"})
		}
		return
	}
	if entry.Mode == "160000" {
		if explicit {
			state.result.Warnings = append(state.result.Warnings, model.Warning{Path: displayPath(state.cwd, path), Message: "Git submodules are not analyzed as files"})
		}
		return
	}
	spec, supported := language.DetectWithSQLDialect(path, state.options.SQLDialect)
	needsShebang := !supported && filepath.Ext(path) == ""
	if !supported && !needsShebang {
		recordUnsupported(path, state.unsupportedSeen, state.unsupportedCounts)
		if explicit {
			state.result.Warnings = append(state.result.Warnings, model.Warning{Path: displayPath(state.cwd, path), Message: "unsupported source extension"})
		}
		return
	}
	if supported && spec.ID == "php" {
		spec, supported = language.DetectWithSourcePrefix(path, state.options.SQLDialect, entry.Content)
		if !supported {
			return
		}
	}
	if needsShebang {
		line := string(entry.Content)
		if index := strings.IndexByte(line, '\n'); index >= 0 {
			line = line[:index]
		}
		spec, supported = language.DetectShebang(strings.TrimSuffix(line, "\r"))
		if !supported {
			recordUnsupported(path, state.unsupportedSeen, state.unsupportedCounts)
			return
		}
	}
	if len(state.options.ComparisonDomains) > 0 {
		if _, selected := state.options.ComparisonDomains[spec.ComparisonDomain]; !selected {
			_, sqlSelected := state.options.ComparisonDomains["sql-query"]
			if !state.options.EmbeddedSQL || !sqlSelected || spec.ID != "go" {
				return
			}
		}
	}
	analysisDomain := spec.ComparisonDomain
	if state.options.EmbeddedSQL && spec.ID == "go" {
		if _, selected := state.options.ComparisonDomains["sql-query"]; selected {
			analysisDomain = "sql-query"
		}
	}
	if state.options.MaxFileBytes > 0 && entry.Size > state.options.MaxFileBytes {
		state.result.Warnings = append(state.result.Warnings, model.Warning{Path: displayPath(state.cwd, path), Message: fmt.Sprintf("file is %d bytes; limit is %d bytes", entry.Size, state.options.MaxFileBytes)})
		state.result.Excluded = append(state.result.Excluded, model.FileCoverage{
			Path: displayPath(state.cwd, path), Language: spec.ID, LanguageFamily: spec.Family,
			ComparisonDomain: analysisDomain, Status: "skipped_resource", ZeroReason: "resource_limit",
		})
		return
	}
	generated, marker := detectGeneratedContent(entry.Content)
	cleanPath := filepath.Clean(path)
	if _, exists := state.seen[cleanPath]; exists {
		return
	}
	state.seen[cleanPath] = struct{}{}
	if generated && state.options.ExcludeGenerated {
		state.result.Excluded = append(state.result.Excluded, model.FileCoverage{
			Path: displayPath(state.cwd, cleanPath), Language: spec.ID, LanguageFamily: spec.Family,
			ComparisonDomain: analysisDomain, Status: "excluded_generated", Generated: true,
			GeneratedMarker: marker, ZeroReason: "generated_excluded",
		})
		return
	}
	content := append([]byte(nil), entry.Content...)
	state.result.Files = append(state.result.Files, File{
		Path: cleanPath, DisplayPath: displayPath(state.cwd, cleanPath), Language: spec,
		AnalysisDomain: analysisDomain, Generated: generated, Marker: marker,
		MaxBytes: state.options.MaxFileBytes, Content: content,
	})
}

func addSnapshotEntry(root *snapshotNode, entry SnapshotEntry) {
	segments := strings.Split(filepath.ToSlash(entry.Path), "/")
	node := root
	for _, segment := range segments[:len(segments)-1] {
		if node.directories[segment] == nil {
			node.directories[segment] = &snapshotNode{directories: map[string]*snapshotNode{}, files: map[string]SnapshotEntry{}}
		}
		node = node.directories[segment]
	}
	node.files[segments[len(segments)-1]] = entry
}

func normalizeSnapshotRoots(root string, cwd string, requested []string, entries []SnapshotEntry) ([]string, map[string]struct{}, error) {
	entrySet := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entrySet[entry.Path] = struct{}{}
	}
	result := make([]string, 0, len(requested))
	explicit := make(map[string]struct{})
	for _, value := range requested {
		absolute := value
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(cwd, value)
		}
		absolute = filepath.Clean(absolute)
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, nil, fmt.Errorf("staged scan path %q is outside the Git worktree", value)
		}
		relative = filepath.ToSlash(relative)
		if relative == "" {
			relative = "."
		}
		if _, ok := entrySet[relative]; ok {
			explicit[relative] = struct{}{}
		}
		result = append(result, relative)
	}
	sort.Strings(result)
	return compactSnapshotStrings(result), explicit, nil
}

func snapshotDirectoryRelevant(directory string, roots []string) bool {
	for _, root := range roots {
		if root == "." || root == directory || strings.HasPrefix(root, directory+"/") || strings.HasPrefix(directory, root+"/") {
			return true
		}
	}
	return false
}

func snapshotDirectoryRequired(directory string, roots []string) bool {
	for _, root := range roots {
		if root == directory || strings.HasPrefix(root, directory+"/") {
			return true
		}
	}
	return false
}

func snapshotFileSelected(path string, roots []string) bool {
	for _, root := range roots {
		if root == "." || path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func compactSnapshotStrings(values []string) []string {
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
