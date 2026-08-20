package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestWriteCompleteJSONReportReplacesRegularFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := model.Report{SchemaVersion: model.SchemaVersion, TotalMatchGroups: 7}
	if err := writeCompleteJSONReport(path, want); err != nil {
		t.Fatalf("writeCompleteJSONReport: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got model.Report
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if got.SchemaVersion != want.SchemaVersion || got.TotalMatchGroups != want.TotalMatchGroups {
		t.Fatalf("report = %#v", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("report mode = %o", info.Mode().Perm())
		}
	}
}

func TestAgentFormatWritesJSONEvidenceOnce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, content := range map[string]string{
		"left.go":  "package sample\nfunc Left(value int) int { return value + 1 }\n",
		"right.go": "package sample\nfunc Right(value int) int { return value + 2 }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	evidencePath := filepath.Join(root, "evidence.json")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--no-config", "--format", "agent", "--output", evidencePath,
		"--min-tokens", "1", root,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("agent scan exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "complete JSON evidence:") ||
		!strings.Contains(stdout.String(), "Mori agent summary:") ||
		!strings.Contains(stdout.String(), "report schema 20") ||
		strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Fatalf("agent output = %q", stdout.String())
	}
	content, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	var result model.Report
	if err := json.Unmarshal(content, &result); err != nil || result.SchemaVersion != model.SchemaVersion || result.TotalMatchGroups != 1 {
		t.Fatalf("evidence report = %#v, err = %v", result, err)
	}
}

func TestReportOutputRequiresAgentFormat(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--no-config", "--format", "json", "--output", filepath.Join(t.TempDir(), "report.json")}, &stdout, &stderr)
	if code != exitUsage || !strings.Contains(stderr.String(), "--output requires --format agent") {
		t.Fatalf("output validation = %d/%q", code, stderr.String())
	}
}

func TestWriteCompleteJSONReportRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available")
	}
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	if err := os.WriteFile(target, []byte("preserve\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "report.json")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := writeCompleteJSONReport(path, model.Report{}); err == nil {
		t.Fatal("expected symlink rejection")
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "preserve\n" {
		t.Fatalf("target changed: %q, %v", content, err)
	}
}
