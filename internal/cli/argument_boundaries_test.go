package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanOptionBoundaries(t *testing.T) {
	argumentTestChdir(t, t.TempDir())
	// Any accidental configuration discovery must fail, even before scanning.
	if err := os.WriteFile(".mori.json", []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		args  []string
		code  int
		paths []string
	}{
		{"misplaced profile", []string{"root", "--profile", "review"}, exitUsage, nil},
		{"misplaced config", []string{"root", "--config=missing"}, exitUsage, nil},
		{"misplaced staged", []string{"root", "--staged"}, exitUsage, nil},
		{"boolean separate value", []string{"--no-config", "false", "--profile=review"}, exitUsage, nil},
		{"terminator as value", []string{"--no-config", "--exclude", "--", "root", "--profile=magic"}, exitUsage, nil},
		{"missing value", []string{"--profile"}, exitUsage, nil},
		{"invalid boolean", []string{"--staged=invalid"}, exitUsage, nil},
		{"literal selectors", []string{"--no-config", "--", "--profile=magic", "--config=missing", "--scope=magic", "--staged", "--no-config"}, exitSuccess, []string{"--profile=magic", "--config=missing", "--scope=magic", "--staged", "--no-config"}},
		{"flag-looking value", []string{"--no-config", "--exclude", "--staged", "root"}, exitSuccess, []string{"root"}},
		{"multiple roots", []string{"--no-config", "one", "two"}, exitSuccess, []string{"one", "two"}},
		{"explicit booleans", []string{"--no-config=false", "--no-config=true", "--staged=false", "root"}, exitSuccess, []string{"root"}},
		{"single dash", []string{"-no-config", "-profile=review", "root"}, exitSuccess, []string{"root"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			_, paths, code, ok := parseScanOptions("scan", test.args, &stderr, "")
			if code != test.code || ok != (test.code == exitSuccess) {
				t.Fatalf("code=%d ok=%t stderr=%s", code, ok, &stderr)
			}
			if strings.Join(paths, "|") != strings.Join(test.paths, "|") {
				t.Fatalf("paths=%q want=%q", paths, test.paths)
			}
		})
	}
}

func TestScanSelectorsUseFinalParsedValues(t *testing.T) {
	argumentTestChdir(t, t.TempDir())
	var stderr bytes.Buffer
	options, _, code, ok := parseScanOptions("scan", []string{"--no-config", "--profile=explore", "--profile", "review", "--threshold", "0.91", "--exclude", "--scope=magic"}, &stderr, "")
	if !ok || code != exitSuccess {
		t.Fatalf("code=%d: %s", code, &stderr)
	}
	if options.profile != "review" || options.threshold != .91 || options.scope != "" || len(options.excludes) != 1 || options.excludes[0] != "--scope=magic" {
		t.Fatalf("options=%+v", options)
	}
}

func TestHelpDoesNotReadProjectOrResolveIndex(t *testing.T) {
	root := t.TempDir()
	argumentTestChdir(t, root)
	if err := os.WriteFile(filepath.Join(root, ".mori.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"inspect", "--help"}, {"doctor", "--help"}, {"scan", "--help"}, {"scan", "--config=missing", "--help"}, {"review", "staged", "check", "--help"}, {"review", "staged", "acknowledge", "--help"}} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), args, &stdout, &stderr)
		if code != exitSuccess || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Usage") {
			t.Fatalf("%v: code=%d stdout=%s stderr=%s", args, code, &stdout, &stderr)
		}
	}
	for _, cmd := range []string{"inspect", "doctor", "scan"} {
		if code := Run(context.Background(), []string{cmd, "--help"}, &bytes.Buffer{}, failingWriter{}); code != exitError {
			t.Fatalf("%s failing writer code=%d", cmd, code)
		}
	}
}

func TestCanonicalReviewRespectsOptionValuesAndLiterals(t *testing.T) {
	for _, args := range [][]string{{"--exclude", "--staged"}, {"--", "--staged"}, {"--exclude=--focus-path"}} {
		if _, err := canonicalStagedReviewArgs(args, "check"); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"--staged=false"}, {"-staged"}, {"root", "--profile=review"}} {
		if _, err := canonicalStagedReviewArgs(args, "check"); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func argumentTestChdir(t *testing.T, path string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
}
