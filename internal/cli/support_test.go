package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/support"
)

func supportTestSession(t *testing.T) (string, support.Session) {
	t.Helper()
	session := support.Session{SchemaVersion: support.SchemaVersion, Tool: support.Build{Version: "dev", Revision: "unknown", GOOS: "darwin", GOARCH: "arm64", GoVersion: "go1.23.0", ReportSchema: 22, NormalizationVersion: 14, ConfigSchema: 2, ContractSchema: 3}, Status: "success", ReportAvailable: true, Phases: []support.Phase{}, Languages: []support.LanguageCount{}}
	path := filepath.Join(t.TempDir(), "session.json")
	if err := support.WriteSession(path, session); err != nil {
		t.Fatal(err)
	}
	return path, session
}

func TestSupportCommandsPreviewExactCanonicalPayload(t *testing.T) {
	path, _ := supportTestSession(t)
	output := filepath.Join(t.TempDir(), "support.zip")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"support", "bundle", "--session", path, "--output", output, "--finding", "7:intentional", "--finding", "2:useful"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("bundle code=%d stderr=%s", code, &stderr)
	}
	var preview support.Session
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.ReviewedFindings) != 2 || preview.ReviewedFindings[0].Rank != 2 {
		t.Fatalf("findings=%+v", preview.ReviewedFindings)
	}
	bundlePreview := stdout.String()
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"support", "inspect", output}, &stdout, &stderr); code != exitSuccess || stdout.String() != bundlePreview {
		t.Fatalf("inspect code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	archive, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 2 {
		t.Fatalf("members=%d", len(archive.File))
	}
	for _, entry := range archive.File {
		if entry.Name != "session.json" && entry.Name != "README.txt" {
			t.Fatalf("unexpected attachment %s", entry.Name)
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"support", "bundle", "--session", path, "--output", output}, &stdout, &stderr); code != exitError {
		t.Fatalf("overwrote bundle code=%d", code)
	}
}

func TestSupportCommandsRejectInvalidAndSensitiveInputs(t *testing.T) {
	path, _ := supportTestSession(t)
	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"secret":"PRIVATE_SOURCE_MARKER"}`), 0600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "linked.json")
	if err := os.Symlink(path, linked); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, args := range [][]string{
		{"support"}, {"support", "unknown"}, {"support", "inspect"}, {"support", "inspect", path, path}, {"support", "inspect", path, "--session", path},
		{"support", "bundle", "--session", path}, {"support", "bundle", "--output", "unused.zip"},
		{"support", "bundle", "--session", invalid, "--output", filepath.Join(t.TempDir(), "invalid.zip")},
		{"support", "inspect", invalid}, {"support", "inspect", linked},
		{"support", "bundle", "--session", path, "--output", filepath.Join(t.TempDir(), "bad.zip"), "--finding", "0:useful"},
		{"support", "bundle", "--session", path, "--output", filepath.Join(t.TempDir(), "bad.zip"), "--finding", "1:free-text"},
		{"support", "bundle", "--session", path, "--output", filepath.Join(t.TempDir(), "bad.zip"), "--finding", "1:useful", "--finding", "1:intentional"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), args, &stdout, &stderr); code == exitSuccess {
			t.Fatalf("accepted %v", args)
		}
		if stdout.Len() != 0 || strings.Contains(stderr.String(), "PRIVATE_SOURCE_MARKER") {
			t.Fatalf("unvalidated input leaked: %s / %s", &stdout, &stderr)
		}
	}
	outside := filepath.Join(t.TempDir(), "keep.zip")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "output.zip")
	if err := os.Symlink(outside, alias); err != nil {
		t.Fatal(err)
	}
	if code := Run(context.Background(), []string{"support", "bundle", "--session", path, "--output", alias}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitError {
		t.Fatalf("symlink output code=%d", code)
	}
	content, err := os.ReadFile(outside)
	if err != nil || string(content) != "keep" {
		t.Fatal("symlink target changed")
	}
}

func TestSupportHelpHasNoInputSideEffects(t *testing.T) {
	for _, args := range [][]string{{"support", "--help"}, {"support", "inspect", "--help"}, {"support", "bundle", "--session", "missing", "--help"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), args, &stdout, &stderr); code != exitSuccess {
			t.Fatalf("%v code=%d stderr=%s", args, code, &stderr)
		}
		if !strings.Contains(stdout.String()+stderr.String(), "Usage:") {
			t.Fatal("missing help")
		}
	}
	if code := Run(context.Background(), []string{"support", "--help"}, failingWriter{}, &bytes.Buffer{}); code != exitError {
		t.Fatalf("writer failure code=%d", code)
	}
}

func TestSupportInspectRejectsUnapprovedArchiveAttachments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attachments.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{"session.json", "README.txt", "private.go"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("PRIVATE_ATTACHMENT_MARKER")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"support", "inspect", path}, &stdout, &stderr); code != exitError {
		t.Fatalf("archive code=%d", code)
	}
	if stdout.Len() != 0 || strings.Contains(stderr.String(), "PRIVATE_ATTACHMENT_MARKER") {
		t.Fatal("unapproved attachment leaked")
	}
}
