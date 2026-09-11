package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/parser"
)

func testParseCache(t *testing.T) *diskParseCache {
	t.Helper()
	return &diskParseCache{directory: t.TempDir(), executable: "test-build", secret: []byte(strings.Repeat("s", 32))}
}
func parseTestKey(s string) string {
	digest := sha256.Sum256([]byte(s))
	return hex.EncodeToString(digest[:])
}
func TestAuthenticatedParseCacheRoundTripAndTampering(t *testing.T) {
	cache := testParseCache(t)
	key := parseTestKey("source")
	entry := parser.CacheEntry{Fragments: []parser.CachedFragment{{Fragment: model.Fragment{TokenCount: 12}, Features: model.FeatureBag{"operation:return": 2}, StartByte: 1, EndByte: 40}}, Warnings: []model.Warning{{Kind: "parse", TotalDiagnostics: 1}}, Coverage: parser.Coverage{CandidateFragments: 2}}
	cache.Store(context.Background(), key, entry)
	got, ok := cache.Load(context.Background(), key)
	if !ok || !reflect.DeepEqual(got, entry) {
		t.Fatalf("round trip=%+v ok=%v", got, ok)
	}
	cache.executable = "new-build"
	if _, ok := cache.Load(context.Background(), key); ok {
		t.Fatal("executable change reused parse")
	}
	cache.executable = "test-build"
	content, err := os.ReadFile(cache.path(key))
	if err != nil {
		t.Fatal(err)
	}
	content[len(content)-2] ^= 1
	if err := os.WriteFile(cache.path(key), content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Load(context.Background(), key); ok {
		t.Fatal("tampering accepted")
	}
}
func TestParseCacheBoundsCancellationAndConcurrency(t *testing.T) {
	cache := testParseCache(t)
	key := parseTestKey("source")
	cache.Store(context.Background(), key, parser.CacheEntry{Warnings: []model.Warning{{Message: strings.Repeat("x", parseCacheMaxBytes)}}})
	if _, err := os.Stat(cache.path(key)); !os.IsNotExist(err) {
		t.Fatalf("oversized entry saved: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cache.Store(ctx, key, parser.CacheEntry{})
	if _, ok := cache.Load(ctx, key); ok {
		t.Fatal("cancelled cache hit")
	}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			cache.Store(context.Background(), key, parser.CacheEntry{Coverage: parser.Coverage{CandidateFragments: 1}})
		}()
	}
	group.Wait()
	if got, ok := cache.Load(context.Background(), key); !ok || got.Coverage.CandidateFragments != 1 {
		t.Fatalf("concurrent write corrupt: %+v %v", got, ok)
	}
	entries, err := os.ReadDir(cache.directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("pending artifacts: %v", entries)
	}
}
func TestParseCacheRejectsSymlinkEntries(t *testing.T) {
	cache := testParseCache(t)
	key := parseTestKey("source")
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, cache.path(key)); err != nil {
		t.Skip(err)
	}
	if _, ok := cache.Load(context.Background(), key); ok {
		t.Fatal("symlink read")
	}
	cache.Store(context.Background(), key, parser.CacheEntry{})
	content, _ := os.ReadFile(target)
	if string(content) != "untouched" {
		t.Fatal("followed symlink on write")
	}
}

func TestDiskParseCachePreservesRealParserInternalEvidence(t *testing.T) {
	cache := testParseCache(t)
	spec, _ := language.Detect("input.js")
	file := source.File{Path: filepath.Join(t.TempDir(), "input.js"), DisplayPath: "input.js", Language: spec, Content: []byte(`function outer(value) { function inner(item) { return item + "different"; } return inner(value); }
function broken( {`)}
	expected, warnings, coverage := parser.FileWithCoverage(context.Background(), file, parser.Options{MinTokens: 1})
	for range 2 {
		got, gotWarnings, gotCoverage := parser.FileWithCoverage(context.Background(), file, parser.Options{MinTokens: 1, Cache: cache})
		if !reflect.DeepEqual(got, expected) || !reflect.DeepEqual(warnings, gotWarnings) || coverage != gotCoverage {
			t.Fatalf("disk cache changed internal evidence: got=%+v want=%+v", got, expected)
		}
	}
}

func TestPlanScanParseCacheJourney(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	root := t.TempDir()
	argumentTestChdir(t, root)
	path := filepath.Join(root, "input.js")
	if err := os.WriteFile(path, []byte(`function first(value) { return value + "first"; }
function second(value) { return value + "second"; }`), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(command string, cached bool) []byte {
		t.Helper()
		args := []string{command, "--no-config", "--format=json", "--min-tokens=1", root}
		if cached {
			args = append(args[:len(args)-1], "--parse-cache", root)
		}
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, &out, &stderr); code != 0 {
			t.Fatalf("%v: %d %s", args, code, stderr.String())
		}
		return out.Bytes()
	}
	cold := run("scan", false)
	run("plan", true)
	cache, err := openParseCache([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(cache.directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatal("plan did not write cache")
	}
	if warm := run("scan", true); !bytes.Equal(cold, warm) {
		t.Fatal("plan cache changed scan output")
	}
	if err := os.WriteFile(path, []byte(`function changed(value) { return value * 7; }`), 0600); err != nil {
		t.Fatal(err)
	}
	fresh, warm := run("scan", false), run("scan", true)
	if !bytes.Equal(fresh, warm) || bytes.Equal(cold, warm) {
		t.Fatal("changed source reused stale parse")
	}
	var report model.Report
	if err := json.Unmarshal(warm, &report); err != nil {
		t.Fatal(err)
	}
}

func TestParseCacheRejectsAuthorityInAliasedRepository(t *testing.T) {
	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	sourceDir := filepath.Join(repository, "src")
	if err := os.Mkdir(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	alias := filepath.Join(external, "source")
	if err := os.Symlink(sourceDir, alias); err != nil {
		t.Skip(err)
	}
	argumentTestChdir(t, external)
	t.Setenv("HOME", repository)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(repository, "cache"))
	t.Setenv("LocalAppData", filepath.Join(repository, "cache"))
	if _, err := openParseCache([]string{alias}); err == nil {
		t.Fatal("repository-controlled cache authority accepted through source alias")
	}
}

func BenchmarkParserDiskCache(b *testing.B) {
	directory := b.TempDir()
	cache := &diskParseCache{directory: directory, executable: "benchmark", secret: []byte(strings.Repeat("s", 32))}
	var code strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&code, "function transform%d(values) { let total = 0; for (const value of values) { if (value > 10) { total += value * 2; } else { total -= value; } } return total; }\n", i)
	}
	spec, _ := language.Detect("input.js")
	file := source.File{Path: filepath.Join(directory, "input.js"), DisplayPath: "input.js", Language: spec, Content: []byte(code.String())}
	parser.FileWithCoverage(context.Background(), file, parser.Options{MinTokens: 1, Cache: cache})
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		b.Fatalf("warm fixture not cached: %v %d", err, len(entries))
	}
	for _, warm := range []bool{false, true} {
		b.Run(fmt.Sprintf("warm=%t", warm), func(b *testing.B) {
			options := parser.Options{MinTokens: 1}
			if warm {
				options.Cache = cache
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				fragments, warnings, _ := parser.FileWithCoverage(context.Background(), file, options)
				if len(fragments) != 40 || len(warnings) != 0 {
					b.Fatal("coverage changed")
				}
			}
		})
	}
}
