package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/vcs"
)

func TestStagedAnalysisCachePreservesReportAndInternalPairs(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("cache requires private POSIX file permissions")
	}
	cache := testStagedCache(t)
	original := model.Report{SchemaVersion: model.SchemaVersion, Files: 4, Warnings: []model.Warning{}, Groups: []model.MatchGroup{{ID: "pair", PathPairs: []model.LocationPair{{Left: model.Location{Path: "a.go"}, Right: model.Location{Path: "b.go"}}}}}}
	if err := cache.Save(original); err != nil {
		t.Fatal(err)
	}
	actual, ok := cache.Load()
	if !ok {
		t.Fatal("cache miss")
	}
	// Empty and nil slices must retain their report representation.
	if !reflect.DeepEqual(original, actual) {
		t.Fatalf("report changed: %+v", actual)
	}
}

func TestStagedAnalysisCacheRejectsModifiedEvidence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache requires private POSIX file permissions")
	}
	t.Parallel()
	for _, kind := range []string{"payload", "key", "authority", "symlink", "oversize", "permissions"} {
		t.Run(kind, func(t *testing.T) {
			cache := testStagedCache(t)
			if err := cache.Save(model.Report{SchemaVersion: model.SchemaVersion}); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "payload":
				content, err := os.ReadFile(cache.path)
				if err != nil {
					t.Fatal(err)
				}
				content[len(content)-1] ^= 1
				if err := os.WriteFile(cache.path, content, 0600); err != nil {
					t.Fatal(err)
				}
			case "key":
				cache.key = "changed"
			case "authority":
				cache.secret = []byte("another authority")
			case "symlink":
				target := cache.path + ".original"
				if err := os.Rename(cache.path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, cache.path); err != nil {
					t.Skip(err)
				}
			case "oversize":
				if err := os.Truncate(cache.path, stagedCacheMaxBytes+33); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.Chmod(cache.path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, ok := cache.Load(); ok {
				t.Fatal("accepted altered cache")
			}
		})
	}
}

func TestStagedAnalysisCacheKeyBindsSnapshotAndOptions(t *testing.T) {
	t.Parallel()
	initial := defaultScanOptions()
	initial.staged = true
	initial.focusedOnly = true
	initial.includeFocused = true
	initial.stagedSnapshot = &vcs.IndexSnapshot{Root: "/project", Prefix: "src", HeadCommit: "head", IndexDigest: "index", Entries: []vcs.IndexEntry{{Path: ".moriignore", OID: "blob"}}}
	key := func(options scanOptions, paths []string, baseline, binary string) string {
		value, err := stagedAnalysisCacheKey(paths, options, baseline, nil, binary)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	original := key(initial, []string{"."}, "baseline", "binary")
	for _, kind := range []string{"threshold", "ignore", "baseline", "binary", "paths", "head", "entry", "prefix", "interval", "policy"} {
		t.Run(kind, func(t *testing.T) {
			changed := initial
			snapshot := *initial.stagedSnapshot
			changed.stagedSnapshot = &snapshot
			paths, baseline, binary := []string{"."}, "baseline", "binary"
			switch kind {
			case "threshold":
				changed.threshold = .9
			case "ignore":
				changed.respectIgnore = false
			case "baseline":
				baseline = "other"
			case "binary":
				binary = "other"
			case "paths":
				paths = []string{"src"}
			case "head":
				snapshot.HeadCommit = "other"
			case "entry":
				snapshot.Entries = []vcs.IndexEntry{{Path: ".moriignore", OID: "other"}}
			case "prefix":
				snapshot.Prefix = "other"
			case "interval":
				snapshot.ChangedLineIntervals = map[string][]model.LineInterval{"x.go": {{StartLine: 1, EndLine: 2}}}
			case "policy":
				changed.reviewPolicy = "advisory"
			}
			if original == key(changed, paths, baseline, binary) {
				t.Fatalf("key ignored %s", kind)
			}
		})
	}
	if original != key(initial, []string{"."}, "baseline", "binary") {
		t.Fatal("nondeterministic key")
	}
}

func TestStagedCacheAuthorityAndConcurrentSave(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache requires private POSIX file permissions")
	}
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "key")
	first, err := stagedCacheSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := stagedCacheSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first) != 32 {
		t.Fatal("authority not stable")
	}
	cache := testStagedCache(t)
	if err := os.WriteFile(cache.path+".pending", []byte("busy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(model.Report{SchemaVersion: model.SchemaVersion}); err == nil {
		t.Fatal("overwrote pending save")
	}
	content, err := os.ReadFile(cache.path + ".pending")
	if err != nil || string(content) != "busy" {
		t.Fatal("changed another writer's pending file")
	}
}

func testStagedCache(t *testing.T) *stagedAnalysisCache {
	t.Helper()
	return &stagedAnalysisCache{path: filepath.Join(t.TempDir(), "report"), key: "input-key", secret: []byte("01234567890123456789012345678901")}
}

func TestStagedAnalysisCacheOpenUsesPrivateWorktreeMetadata(t *testing.T) {
	// UserConfigDir is process-global; do not run this test in parallel.
	authority, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", authority)
	t.Setenv("APPDATA", filepath.Join(authority, "appdata"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(authority, "config"))
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc Value() int { return 1 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", "sample.go")
	snapshot, err := vcs.ResolveIndex(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	options := defaultScanOptions()
	options.staged = true
	options.stagedSnapshot = &snapshot
	options.focusedOnly = true
	options.includeFocused = true
	cache, err := openStagedAnalysisCache(context.Background(), []string{root}, options, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := executeScan(context.Background(), []string{root}, options, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(report); err != nil {
		t.Fatal(err)
	}
	options.outputPath = "another-report.json"
	options.format = "agent"
	options.redactPaths = true
	again, err := openStagedAnalysisCache(context.Background(), []string{root}, options, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := again.Load()
	originalJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	loadedJSON, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !bytes.Equal(originalJSON, loadedJSON) {
		t.Fatal("cache did not survive reopening")
	}
	// An unstaged source edit is outside the immutable staged input.
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("broken unstaged content"), 0600); err != nil {
		t.Fatal(err)
	}
	unchanged, err := vcs.ResolveIndex(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	options.stagedSnapshot = &unchanged
	again, err = openStagedAnalysisCache(context.Background(), []string{root}, options, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Load(); !ok {
		t.Fatal("unstaged edit invalidated index cache")
	}
	runTestGit(t, root, "add", "sample.go")
	changed, err := vcs.ResolveIndex(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	options.stagedSnapshot = &changed
	again, err = openStagedAnalysisCache(context.Background(), []string{root}, options, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Load(); ok {
		t.Fatal("staged edit reused stale cache")
	}
}

func TestStagedAnalysisCacheRecoversOnlyStalePrivatePending(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache requires private POSIX permissions")
	}
	t.Parallel()
	for _, kind := range []string{"stale", "recent", "symlink", "public"} {
		t.Run(kind, func(t *testing.T) {
			cache := testStagedCache(t)
			pending := cache.path + ".pending"
			if err := os.WriteFile(pending, []byte("orphan"), 0600); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-11 * time.Minute)
			if kind != "recent" {
				if err := os.Chtimes(pending, old, old); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symlink" {
				target := pending + ".target"
				if err := os.Rename(pending, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, pending); err != nil {
					t.Skip(err)
				}
			}
			if kind == "public" {
				if err := os.Chmod(pending, 0644); err != nil {
					t.Fatal(err)
				}
			}
			err := cache.Save(model.Report{SchemaVersion: model.SchemaVersion})
			if kind == "stale" {
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := cache.Load(); !ok {
					t.Fatal("recovered save missed")
				}
				return
			}
			if err == nil {
				t.Fatal("replaced protected pending file")
			}
			content, err := os.ReadFile(pending)
			if err != nil || string(content) != "orphan" {
				t.Fatal("changed protected pending file")
			}
		})
	}
}

func TestStagedAnalysisCachePresentationDoesNotInvalidate(t *testing.T) {
	t.Parallel()
	options := defaultScanOptions()
	first, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	options.outputPath = "other-report.json"
	options.format = "agent"
	options.color = "never"
	options.redactPaths = true
	options.reviewReceiptPath = "other-receipt.json"
	second, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("render-only inputs invalidated analysis")
	}
	options.maxGroups = 0
	third, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	if third == second {
		t.Fatal("receipt-driven completeness must invalidate analysis")
	}
}

func TestStagedCacheLocationDictionaryPreservesPairOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache requires private POSIX permissions")
	}
	t.Parallel()
	cache := testStagedCache(t)
	left := model.Location{Path: "a.go", Name: "Alpha", StartLine: 1, EndLine: 20}
	right := model.Location{Path: "b.go", Name: "Beta", StartLine: 3, EndLine: 22}
	original := model.Report{SchemaVersion: model.SchemaVersion, Groups: []model.MatchGroup{
		{PathPairs: []model.LocationPair{{Left: left, Right: right}, {Left: right, Right: left}, {Left: left, Right: right}}},
		{PathPairs: []model.LocationPair{}}, {},
	}}
	if err := cache.Save(original); err != nil {
		t.Fatal(err)
	}
	loaded, ok := cache.Load()
	if !ok || !reflect.DeepEqual(original, loaded) {
		t.Fatal("dictionary changed pair ordering or empty slices")
	}
	content, err := os.ReadFile(cache.path)
	if err != nil {
		t.Fatal(err)
	}
	var stored stagedCachePayload
	if err := json.Unmarshal(content[sha256.Size:], &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Locations) != 2 {
		t.Fatalf("dictionary has %d locations", len(stored.Locations))
	}
	for _, invalid := range []int{-1, len(stored.Locations)} {
		stored.PathPairs[0][0][0] = invalid
		payload, err := json.Marshal(stored)
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, cache.secret)
		mac.Write([]byte(cache.key))
		mac.Write(payload)
		if err := os.WriteFile(cache.path, append(mac.Sum(nil), payload...), 0600); err != nil {
			t.Fatal(err)
		}
		if _, ok := cache.Load(); ok {
			t.Fatal("accepted invalid dictionary reference")
		}
	}
}

func TestStagedCacheLocationDictionaryKeepsLargeRepeatedReport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cache requires private POSIX permissions")
	}
	t.Parallel()
	cache := testStagedCache(t)
	pair := model.LocationPair{Left: model.Location{Path: "left.go"}, Right: model.Location{Path: "right.go"}}
	pairs := make([]model.LocationPair, 80_000)
	for i := range pairs {
		pairs[i] = pair
	}
	report := model.Report{SchemaVersion: model.SchemaVersion, Groups: []model.MatchGroup{{PathPairs: pairs}}}
	if err := cache.Save(report); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cache.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1_000_000 {
		t.Fatalf("repeated pair cache unexpectedly large: %d", info.Size())
	}
	loaded, ok := cache.Load()
	if !ok || !reflect.DeepEqual(report, loaded) {
		t.Fatal("large report did not round trip")
	}
}
