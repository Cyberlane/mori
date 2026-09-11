package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/support"
)

func TestDiagnosticsClassificationCountsWithoutPaths(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "private-source-sentinel.go")
	if err := os.WriteFile(source, []byte("package fixture\nfunc add(value int) int { return value + 1 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(root, "session.json")
	var out, stderr bytes.Buffer
	args := []string{"scan", "--no-config", "--min-tokens=1", "--diagnostics", sessionPath, "--production-path", filepath.Join(root, "private-production-sentinel"), "--test-path", filepath.Join(root, "private-test-sentinel"), source}
	if code := Run(context.Background(), args, &out, &stderr); code != exitSuccess {
		t.Fatalf("%d %s", code, stderr.String())
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), root) || strings.Contains(string(data), "private-") {
		t.Fatal("diagnostics leaked classification paths")
	}
	var decoded struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"production_path_count", "test_path_count"} {
		if decoded.Settings[name] != float64(1) {
			t.Errorf("%s = %v", name, decoded.Settings[name])
		}
	}
	if _, err := support.ReadSession(sessionPath); err != nil {
		t.Fatalf("new diagnostic reader: %v", err)
	}
}

func TestClassificationPathOperandsAreNotOptionSelectors(t *testing.T) {
	argumentTestChdir(t, t.TempDir())
	for _, flag := range []string{"--production-path", "--test-path"} {
		var stderr bytes.Buffer
		options, paths, code, ok := parseScanOptions("scan", []string{"--no-config", flag, "--staged", "source"}, &stderr, "")
		if !ok || code != exitSuccess || options.staged || len(paths) != 1 || paths[0] != "source" {
			t.Fatalf("%s: %d %v %v %s", flag, code, ok, paths, stderr.String())
		}
		if _, err := canonicalStagedReviewArgs([]string{flag, "--staged"}, "check"); err != nil {
			t.Fatalf("path operand interpreted as staged option: %v", err)
		}
	}
}

func TestParseCacheBooleanDoesNotConsumeSourceOperand(t *testing.T) {
	argumentTestChdir(t, t.TempDir())
	var stderr bytes.Buffer
	options, paths, code, ok := parseScanOptions("scan", []string{"--no-config", "--parse-cache", "source"}, &stderr, "")
	if !ok || code != exitSuccess || !options.parseCache || len(paths) != 1 || paths[0] != "source" {
		t.Fatalf("cache option boundary: %d %v %v %s", code, ok, paths, stderr.String())
	}
}
