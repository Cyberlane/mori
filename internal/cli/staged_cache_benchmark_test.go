package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/vcs"
)

// A fixed synthetic fixture measures analysis separately from report reuse.
// The executable digest benchmark accounts for the per-process hashing cost
// excluded by the in-process warm benchmark's sync.Once identity cache.
func BenchmarkStagedAnalysisReuse(b *testing.B) {
	for _, size := range []int{80, 400} {
		b.Run(fmt.Sprint(size), func(b *testing.B) { benchmarkStagedAnalysisReuse(b, size) })
	}
}

func benchmarkStagedAnalysisReuse(b *testing.B, size int) {
	authority, err := filepath.EvalSymlinks(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Setenv("HOME", authority)
	b.Setenv("APPDATA", filepath.Join(authority, "appdata"))
	b.Setenv("XDG_CONFIG_HOME", filepath.Join(authority, "config"))
	root, err := filepath.EvalSymlinks(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	git := func(args ...string) {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			b.Fatalf("git: %v %s", err, output)
		}
	}
	git("init")
	var content strings.Builder
	content.WriteString("package fixture\n")
	for index := 0; index < size; index++ {
		fmt.Fprintf(&content, "func Function%d(values []int) int { total := 0; for _, value := range values { if value > %d { total += value } }; return total }\n", index, index)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture.go"), []byte(content.String()), 0600); err != nil {
		b.Fatal(err)
	}
	git("add", "fixture.go")
	snapshot, err := vcs.ResolveIndex(context.Background(), root)
	if err != nil {
		b.Fatal(err)
	}
	options := defaultScanOptions()
	options.staged = true
	options.stagedSnapshot = &snapshot
	options.focusedOnly = true
	options.includeFocused = true
	options.minTokens = 1
	paths := []string{root}
	report, err := executeScan(context.Background(), paths, options, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	cache, err := openStagedAnalysisCache(context.Background(), paths, options, "", nil)
	if err != nil {
		b.Fatal(err)
	}
	saveErr := cache.Save(report)
	b.Run("analysis", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := executeScan(context.Background(), paths, options, nil, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("reuse", func(b *testing.B) {
		if saveErr != nil {
			b.Skipf("cache storage bound: %v", saveErr)
		}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			opened, err := openStagedAnalysisCache(context.Background(), paths, options, "", nil)
			if err != nil {
				b.Fatal(err)
			}
			if _, ok := opened.Load(); !ok {
				b.Fatal("cache miss")
			}
		}
	})
}

func BenchmarkStagedCacheExecutableHash(b *testing.B) {
	path, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(info.Size())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		file, err := os.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		closeErr := file.Close()
		if err != nil {
			b.Fatal(err)
		}
		if closeErr != nil {
			b.Fatal(closeErr)
		}
	}
}
