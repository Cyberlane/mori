package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestCachedStagedCheckPreservesOutputAndRevalidatesReceipt(t *testing.T) {
	configRoot := t.TempDir()
	configRoot, err := filepath.EvalSymlinks(configRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("HOME", configRoot)
	t.Setenv("APPDATA", filepath.Join(configRoot, "appdata"))
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	cliTestGit(t, root, "config", "user.name", "Mori Test")
	cliTestGit(t, root, "config", "user.email", "mori@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "README.md")
	cliTestGit(t, root, "commit", "-m", "base")
	for name, fn := range map[string]string{"a.go": "First", "b.go": "Second"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package sample\nfunc "+fn+"(x int) int { return x+1 }\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cliTestGit(t, root, "add", ".")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	run := func(extra ...string) (int, string, string) {
		t.Helper()
		args := append([]string{"review", "staged", "check", "--no-config", "--format", "json", "--min-tokens", "1"}, extra...)
		args = append(args, ".")
		var out, errOut bytes.Buffer
		code := Run(context.Background(), args, &out, &errOut)
		return code, out.String(), errOut.String()
	}
	coldCode, cold, coldErr := run()
	firstCode, first, firstErr := run("--cache")
	secondCode, second, secondErr := run("--cache")
	if coldCode != exitFindings || firstCode != coldCode || secondCode != coldCode || cold != first || cold != second || coldErr != firstErr || coldErr != secondErr {
		t.Fatal("cache changed result")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "mori", "analysis-cache-v1", "report")); err != nil {
		t.Fatal("cache was not written", err)
	}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"review", "staged", "acknowledge", "--no-config", "--min-tokens", "1", "--accept-focused", "."}, &out, &errOut); code != exitSuccess {
		t.Fatalf("acknowledge %d %s", code, &errOut)
	}
	receipt := filepath.Join(root, ".git", "mori", "staged-review.json")
	for i := 0; i < 2; i++ {
		code, content, why := run("--cache", "--review-receipt", receipt)
		if code != exitSuccess {
			t.Fatalf("receipt %d %s", code, why)
		}
		var r model.Report
		if err := json.Unmarshal([]byte(content), &r); err != nil || !r.Review.Acknowledged {
			t.Fatalf("missing receipt evidence %v", err)
		}
	}
	if err := os.WriteFile(receipt, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("--cache", "--review-receipt", receipt); code != exitError {
		t.Fatalf("tampered receipt reused: %d", code)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.go"), []byte("package sample\nfunc broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "bad.go")
	if code, _, _ := run("--cache", "--policy", "advisory", "--fail-on-parse-diagnostic"); code != exitCoverage {
		t.Fatalf("cache bypassed changed-source coverage: %d", code)
	}
}
