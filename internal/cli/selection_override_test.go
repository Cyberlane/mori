package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Cyberlane/mori/internal/baseline"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise discovery, config, parsing, selection and plan through the CLI.
func TestShippedTestLibraryClassificationJourney(t *testing.T) {
	root := t.TempDir()
	argumentTestChdir(t, root)
	if err := os.MkdirAll("library/test", 0700); err != nil {
		t.Fatal(err)
	}
	source := []byte("def request(value):\n    return value + 1\n\ndef client(value):\n    return value + 2\n")
	if err := os.WriteFile("library/test/client.py", source, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".mori.json", []byte(`{"min_tokens":1,"fragment_selection":"production","production_paths":["library/test"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"scan", "plan"} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), []string{command, "--format=json", "library"}, &out, &stderr)
		if code != 0 {
			t.Fatalf("%s: %d %s", command, code, stderr.String())
		}
		var result struct {
			Fragments     int `json:"fragments"`
			Configuration struct {
				ProductionPaths []string `json:"production_paths"`
			} `json:"configuration"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Fragments != 2 || len(result.Configuration.ProductionPaths) != 1 {
			t.Fatalf("%s: %+v", command, result)
		}
	}
	var out, stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--no-config", "--fragment-selection=production", "--min-tokens=1", "--format=json", "library"}, &out, &stderr)
	if code != 0 {
		t.Fatalf("default scan: %d %s", code, stderr.String())
	}
	var result struct {
		Fragments int `json:"fragments"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Fragments != 0 {
		t.Fatalf("default classifier changed: %s %v", out.String(), err)
	}
	out.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{"scan", "--test-path", "library", "library"}, &out, &stderr)
	if code != exitUsage {
		t.Fatalf("overlap must reject: %d %s", code, stderr.String())
	}
}

func TestClassificationPolicyInvalidatesBaselineAndStagedCache(t *testing.T) {
	argumentTestChdir(t, t.TempDir())
	original := defaultScanOptions()
	profile, err := baselineScanProfile(original, nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := stagedAnalysisCacheKey([]string{"."}, original, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--production-path=library/test", "--test-path=library/support"} {
		var stderr bytes.Buffer
		changed, _, _, ok := parseScanOptions("scan", []string{"--no-config", flag}, &stderr, "")
		if !ok {
			t.Fatal(stderr.String())
		}
		next, err := baselineScanProfile(changed, nil)
		if err != nil {
			t.Fatal(err)
		}
		if baseline.Digest(profile) == baseline.Digest(next) {
			t.Fatal("classification reused baseline identity")
		}
		nextKey, err := stagedAnalysisCacheKey([]string{"."}, changed, "", nil, "binary")
		if err != nil {
			t.Fatal(err)
		}
		if key == nextKey {
			t.Fatal("classification reused cache identity")
		}
	}
}

func TestClassificationOverrideConfigBaseAndRedaction(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "library", "test"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "library", "test", "client.py"), []byte("def client(value):\n    return value + 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "policy.json"), []byte(`{"min_tokens":1,"fragment_selection":"production","production_paths":["../library/test"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	argumentTestChdir(t, root)
	var out, stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--config=config/policy.json", "--redact-paths", "--format=json", "library"}, &out, &stderr)
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if strings.Contains(out.String(), "library/test") || strings.Contains(out.String(), root) {
		t.Fatal("classification path leaked through redaction")
	}
	var result struct {
		Fragments int `json:"fragments"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Fragments != 1 {
		t.Fatalf("wrong config-relative classification: %s %v", out.String(), err)
	}
}

func TestEquivalentClassificationPathsPreserveBaselineIdentity(t *testing.T) {
	root := t.TempDir()
	argumentTestChdir(t, root)
	if err := os.MkdirAll("library/test", 0700); err != nil {
		t.Fatal(err)
	}
	var want string
	for _, path := range []string{"library/test", "./library/test", filepath.Join(root, "library/test")} {
		options := defaultScanOptions()
		options.productionPaths = []string{path}
		profile, err := baselineScanProfile(options, nil)
		if err != nil {
			t.Fatal(err)
		}
		digest := baseline.Digest(profile)
		if want != "" && digest != want {
			t.Fatalf("equivalent path changed profile: %s", path)
		}
		want = digest
	}
}

func TestClassificationUsesStagedPolicyAndSource(t *testing.T) {
	root := t.TempDir()
	argumentTestChdir(t, root)
	cliTestGit(t, root, "init", "--initial-branch=main")
	cliTestGit(t, root, "config", "user.name", "Mori Test")
	cliTestGit(t, root, "config", "user.email", "mori@example.invalid")
	if err := os.WriteFile("README.md", []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "README.md")
	cliTestGit(t, root, "commit", "-m", "base")
	if err := os.MkdirAll("library/test", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("library/test/client.py", []byte("def first(value):\n    return value + 1\n\ndef second(value):\n    return value + 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".mori.json", []byte(`{"min_tokens":1,"fragment_selection":"production","production_paths":["library/test"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", ".")
	// Neither a later working config nor malformed working source may replace
	// the immutable index used by the staged gate.
	if err := os.WriteFile(".mori.json", []byte(`{"fragment_selection":"tests"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("library/test/client.py", []byte("def broken(:"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := Run(context.Background(), []string{"review", "staged", "check", "--policy=advisory", "--format=json", "."}, &out, &stderr)
	if code != 0 {
		t.Fatalf("staged %d %s", code, stderr.String())
	}
	var report struct {
		Fragments     int `json:"fragments"`
		Configuration struct {
			ProductionPaths []string `json:"production_paths"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Fragments != 2 || len(report.Configuration.ProductionPaths) != 1 {
		t.Fatalf("working tree replaced staged policy: %s", out.String())
	}
}

func TestScopeCanClearClassificationOverrides(t *testing.T) {
	argumentTestChdir(t, t.TempDir())
	if err := os.WriteFile(".mori.json", []byte(`{"production_paths":["library/test"],"scopes":{"inclusive":{"roots":["."],"production_paths":[]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	options, _, _, ok := parseScanOptions("scan", []string{"--scope=inclusive"}, &stderr, "")
	if !ok || len(options.productionPaths) != 0 {
		t.Fatalf("scope did not clear override: %+v %s", options.productionPaths, stderr.String())
	}
}

func TestSelectionAliasRetargetInvalidatesStagedCache(t *testing.T) {
	root := t.TempDir()
	argumentTestChdir(t, root)
	for _, directory := range []string{"one", "two"} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("one", "alias"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	options := defaultScanOptions()
	options.productionPaths = []string{"alias"}
	first, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("alias"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("two", "alias"); err != nil {
		t.Fatal(err)
	}
	second, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("changed effective classification reused staged cache identity")
	}
}
