package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestExplainSelectsOneContentPairAsHTML(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, content := range map[string]string{
		"a.go": "package sample\nfunc alpha(value int) int { return value + 1 }\n",
		"b.go": "package sample\nfunc beta(item int) int { return item + 1 }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var scanOut, scanErr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--format", "json", "--threshold", "1", "--min-tokens", "1", root}, &scanOut, &scanErr)
	if code != exitSuccess {
		t.Fatalf("scan = %d, stderr = %q", code, scanErr.String())
	}
	var result model.Report
	if err := json.Unmarshal(scanOut.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("groups = %d", len(result.Groups))
	}
	var stdout, stderr bytes.Buffer
	code = Run(context.Background(), []string{"explain", result.Groups[0].ID, "--format", "html", "--threshold", "1", "--min-tokens", "1", root}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("explain = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "<!doctype html>") || !strings.Contains(stdout.String(), result.Groups[0].ID) {
		t.Fatalf("HTML = %q", stdout.String())
	}
}

func TestScanColorAlwaysAndNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\nfunc alpha() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var colored, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"scan", "--color", "always", "--min-tokens", "1", root}, &colored, &stderr); code != exitSuccess {
		t.Fatalf("scan = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Fatalf("color output = %q", colored.String())
	}
	t.Setenv("NO_COLOR", "1")
	var plain bytes.Buffer
	stderr.Reset()
	if code := Run(context.Background(), []string{"scan", "--color", "always", "--min-tokens", "1", root}, &plain, &stderr); code != exitSuccess {
		t.Fatalf("scan = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("NO_COLOR output = %q", plain.String())
	}
}
