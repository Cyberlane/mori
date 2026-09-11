package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFragmentSelectionChangesStagedCacheIdentity(t *testing.T) {
	options := defaultScanOptions()
	keys := map[string]string{}
	for _, selection := range []string{"", "production", "tests"} {
		options.fragmentSelection = selection
		key, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "test-executable")
		if err != nil {
			t.Fatal(err)
		}
		for other, previous := range keys {
			if key == previous {
				t.Fatalf("%q shares cache identity with %q", selection, other)
			}
		}
		keys[selection] = key
	}
}

func TestCandidateLimitCannotCreateAcceptanceOrCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cliTestGit(t, root, "init", "--initial-branch=main")
	cliTestGit(t, root, "config", "user.name", "Mori Test")
	cliTestGit(t, root, "config", "user.email", "mori@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "README.md")
	cliTestGit(t, root, "commit", "-m", "base")
	source := "package sample\nfunc First(x int) int { return x+1 }\nfunc Second(x int) int { return x+2 }\nfunc Third(x int) int { return x+3 }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "sample.go")
	argumentTestChdir(t, root)
	baselinePath := filepath.Join(root, "accepted.json")
	for _, args := range [][]string{
		{"review", "staged", "check", "--cache", "--no-config", "--min-tokens=1", "--max-pairs=1", "--format=json"},
		{"review", "staged", "acknowledge", "--accept-focused", "--no-config", "--min-tokens=1", "--max-pairs=1"},
		{"baseline", "update", "--accept-all", "--baseline", baselinePath, "--no-config", "--min-tokens=1", "--max-pairs=1"},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), args, &stdout, &stderr)
		if code != exitError || !strings.Contains(stderr.String(), "candidate pair limit") {
			t.Fatalf("%v: code=%d stdout=%s stderr=%s", args, code, &stdout, &stderr)
		}
		for _, path := range []string{baselinePath, filepath.Join(root, ".git", "mori", "staged-review.json"), filepath.Join(root, ".git", "mori", "analysis-cache-v1", "report")} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("incomplete analysis wrote acceptance/cache %s: %v", path, err)
			}
		}
	}
}
