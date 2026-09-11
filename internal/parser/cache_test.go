package parser

import (
	"context"
	"reflect"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

type memoryParseCache struct {
	entries      map[string]CacheEntry
	hits, stores int
}

func (cache *memoryParseCache) Load(_ context.Context, key string) (CacheEntry, bool) {
	entry, ok := cache.entries[key]
	if ok {
		cache.hits++
	}
	return entry, ok
}
func (cache *memoryParseCache) Store(_ context.Context, key string, entry CacheEntry) {
	cache.entries[key] = entry
	cache.stores++
}

func TestParseCachePreservesFeaturesWarningsAndInvalidation(t *testing.T) {
	spec, _ := language.Detect("input.js")
	file := source.File{Path: "input.js", DisplayPath: "input.js", Language: spec, Content: []byte("function outer(value) { function inner(value) { return value + 1; } return inner(value); }\nfunction broken( {\n")}
	options := Options{MinTokens: 1}
	expected, warnings, coverage := FileWithCoverage(context.Background(), file, options)
	cache := &memoryParseCache{entries: map[string]CacheEntry{}}
	options.Cache = cache
	for range 2 {
		got, gotWarnings, gotCoverage := FileWithCoverage(context.Background(), file, options)
		if !reflect.DeepEqual(got, expected) || !reflect.DeepEqual(gotWarnings, warnings) || gotCoverage != coverage {
			t.Fatal("cache changed features, nesting or coverage")
		}
	}
	if cache.hits != 1 || cache.stores != 1 {
		t.Fatalf("hits=%d stores=%d", cache.hits, cache.stores)
	}
	file.Content = []byte("function changed(value) { return value * 2; }")
	FileWithCoverage(context.Background(), file, options)
	options.MinTokens = 100
	FileWithCoverage(context.Background(), file, options)
	options.FragmentSelection = "tests"
	FileWithCoverage(context.Background(), file, options)
	if cache.hits != 1 || cache.stores != 4 {
		t.Fatalf("source/settings did not invalidate: hits=%d stores=%d", cache.hits, cache.stores)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	FileWithCoverage(ctx, file, options)
	if cache.hits != 1 || cache.stores != 4 {
		t.Fatal("cancelled parsing used cache")
	}
}
