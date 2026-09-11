package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Warm extraction must not skip fresh discovery or any downstream policy gate.
func TestWarmParseCacheReevaluatesDiscoveryAndPolicies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	root := t.TempDir()
	argumentTestChdir(t, root)
	if err := os.MkdirAll("test", 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"input.js", "test/support.js"} {
		if err := os.WriteFile(path, []byte(`function first(value) { return value + "first"; }
function second(value) { return value + "second"; }`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(cached bool, extra ...string) (int, string, string) {
		t.Helper()
		args := []string{"scan", "--no-config", "--format=json", "--min-tokens=1"}
		if cached {
			args = append(args, "--parse-cache")
		}
		args = append(args, extra...)
		args = append(args, ".")
		var out, stderr bytes.Buffer
		code := Run(context.Background(), args, &out, &stderr)
		return code, out.String(), stderr.String()
	}
	if code, _, why := run(true); code != 0 {
		t.Fatalf("warm up %d %s", code, why)
	}
	check := func(want int, extra ...string) {
		t.Helper()
		cold, out, errOut := run(false, extra...)
		warm, cached, cacheErr := run(true, extra...)
		if cold != want || warm != cold || cached != out || cacheErr != errOut {
			t.Fatalf("cache changed policy %v: cold=%d warm=%d errors=%s / %s", extra, cold, warm, errOut, cacheErr)
		}
	}
	check(exitFindings, "--fail-on-match")
	check(exitError, "--max-pairs=1")
	check(exitSuccess, "--fragment-selection=production")
	check(exitSuccess, "--fragment-selection=production", "--production-path=test")
	if err := os.WriteFile(".moriignore", []byte("test/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	check(exitSuccess)
	if err := os.WriteFile("bad.js", []byte("function broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	check(exitCoverage, "--fail-on-parse-diagnostic")
	// Cached diagnostics must also survive another invocation.
	check(exitCoverage, "--fail-on-parse-diagnostic")
}
