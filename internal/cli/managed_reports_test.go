package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func managedReportRun(t *testing.T, directory string, args ...string) (int, string, string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func managedReportFiles(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if managedReportName.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	return names
}

func managedReportRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\nfunc First(x int) int { return x+1 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "a.go")
	cliTestGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "base")
	return root
}

func TestManagedOutputWritesUnderGitCommonDirectory(t *testing.T) {
	root := managedReportRepository(t)
	code, out, errOut := managedReportRun(t, root, "scan", "--no-config", "--format", "agent", "--output", "auto", ".")
	if code != exitSuccess {
		t.Fatalf("%d: %s %s", code, out, errOut)
	}
	reports := filepath.Join(root, ".git", "mori", "reports")
	names := managedReportFiles(t, reports)
	if len(names) != 1 || !strings.HasPrefix(names[0], "scan-") || !strings.Contains(out, "complete JSON evidence: ") || !strings.Contains(out, names[0]) {
		t.Fatalf("managed report %v: %s", names, out)
	}
	info, err := os.Stat(reports)
	if err != nil || !info.IsDir() {
		t.Fatalf("managed directory: %v %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("managed directory mode: %v", info.Mode())
	}

	linked := filepath.Join(t.TempDir(), "linked")
	cliTestGit(t, root, "worktree", "add", "-q", linked)
	if code, out, errOut := managedReportRun(t, linked, "scan", "--no-config", "--format", "agent", "--output", "auto", "a.go"); code != exitSuccess {
		t.Fatalf("worktree %d: %s %s", code, out, errOut)
	}
	if names := managedReportFiles(t, reports); len(names) != 2 {
		t.Fatalf("linked worktree report not in common directory: %v", names)
	}
}

func TestManagedOutputRotationKeepsNewestAndForeignFiles(t *testing.T) {
	root := managedReportRepository(t)
	reports := filepath.Join(root, ".git", "mori", "reports")
	if err := os.MkdirAll(reports, 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < managedReportsKept+5; i++ {
		label := []string{"scan", "staged-check"}[i%2]
		name := fmt.Sprintf("%s-%s.json", label, base.Add(time.Duration(i)*time.Minute).Format("20060102T150405.000000000Z"))
		if err := os.WriteFile(filepath.Join(reports, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	foreign := []string{"notes.json", "scan-latest.json"}
	for _, name := range foreign {
		if err := os.WriteFile(filepath.Join(reports, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if code, out, errOut := managedReportRun(t, root, "scan", "--no-config", "--format", "agent", "--output", "auto", "."); code != exitSuccess {
		t.Fatalf("%d: %s %s", code, out, errOut)
	}
	names := managedReportFiles(t, reports)
	if len(names) != managedReportsKept {
		t.Fatalf("kept %d: %v", len(names), names)
	}
	// 25 seeded + 1 new report leaves room for 20: the six oldest are pruned.
	oldest := fmt.Sprintf("-%s.json", base.Add(6*time.Minute).Format("20060102T150405.000000000Z"))
	found := false
	for _, name := range names {
		if strings.HasPrefix(name, "scan-2020") || strings.HasPrefix(name, "staged-check-2020") {
			if strings.HasSuffix(name, oldest) {
				found = true
			}
			for i := 0; i < 6; i++ {
				if strings.HasSuffix(name, fmt.Sprintf("-%s.json", base.Add(time.Duration(i)*time.Minute).Format("20060102T150405.000000000Z"))) {
					t.Fatalf("oldest report %s was retained", name)
				}
			}
		}
	}
	if !found {
		t.Fatalf("seventh-oldest report was pruned: %v", names)
	}
	for _, name := range foreign {
		if _, err := os.Stat(filepath.Join(reports, name)); err != nil {
			t.Fatalf("foreign file %s touched: %v", name, err)
		}
	}
}

func TestManagedOutputRequiresGitAndAgentFormat(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "a.go"), []byte("package sample\nfunc First(x int) int { return x+1 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(directory))
	code, _, errOut := managedReportRun(t, directory, "scan", "--no-config", "--format", "agent", "--output", "auto", ".")
	if code != exitUsage || !strings.Contains(errOut, "--output auto requires a Git repository") {
		t.Fatalf("non-Git: %d %s", code, errOut)
	}
	root := managedReportRepository(t)
	if code, _, _ := managedReportRun(t, root, "scan", "--no-config", "--format", "text", "--output", "auto", "."); code != exitUsage {
		t.Fatalf("text format accepted --output auto: %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "mori", "reports")); !os.IsNotExist(err) {
		t.Fatalf("rejected invocation created managed directory: %v", err)
	}
}
