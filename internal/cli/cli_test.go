package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/baseline"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestPriorityPathRulesValidateAndSort(t *testing.T) {
	t.Parallel()

	rules, err := parsePriorityPathRules([]string{"**/z/**=5", "**/a/**=25"})
	if err != nil {
		t.Fatalf("parsePriorityPathRules: %v", err)
	}
	if len(rules) != 2 || rules[0].Pattern != "**/a/**" || rules[0].Priority != 25 {
		t.Fatalf("rules = %#v", rules)
	}
	for _, values := range [][]string{
		{"missing-weight"},
		{"**/auth/**=0"},
		{"**/auth/**=101"},
		{"[=10"},
		{"**/auth/**=10", "**/auth/**=20"},
	} {
		if _, err := parsePriorityPathRules(values); err == nil {
			t.Fatalf("parsePriorityPathRules(%#v) succeeded", values)
		}
	}
}

func TestRunLanguages(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"languages"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	for _, expected := range []string{"FRAGMENTS", "Bash / POSIX shell", "Go", "Hack", "JavaScript / JSX", "PHP", "PostgreSQL queries", "Python", "Rust", "SQL queries", "Swift", "TypeScript / TSX", "Zsh", "shell", "sql-query", "function, script", "bash, dash, sh"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("languages output missing %q", expected)
		}
	}
}

func TestRunScanPostgreSQLQueries(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--sql-dialect", "postgresql",
		"--comparison-domain", "sql-query", "--threshold", "0.90", "--min-tokens", "12",
		"../../examples/postgresql-queries",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.Files != 3 || result.Fragments != 3 || result.TotalMatchGroups != 1 ||
		len(result.Groups) != 1 || len(result.Warnings) != 0 ||
		result.Configuration.SQLDialect != "postgresql" {
		t.Fatalf("PostgreSQL report summary = %+v", result)
	}
	for _, profile := range result.Groups[0].Profiles {
		for _, occurrence := range profile.Occurrences {
			if occurrence.Location.Language != "postgresql" ||
				occurrence.Location.LanguageFamily != "sql" {
				t.Fatalf("PostgreSQL location = %+v", occurrence.Location)
			}
		}
	}
}

func TestRunScanEmbeddedGoSQLIsOptInAndSourceMapped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files := map[string]string{
		"users.go":   "package sample\nfunc users(db *sql.DB) { db.Query(`SELECT id FROM users WHERE tenant_id = $1`) }\n",
		"folders.go": "package sample\nfunc folders(db *sql.DB) { db.Query(`SELECT id FROM folders WHERE tenant_id = $1`) }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"scan", "--format", "json", "--comparison-domain", "sql-query",
		"--embedded-sql", "--sql-dialect", "postgresql", "--threshold", "1",
		"--min-tokens", "1", root,
	}
	code := Run(context.Background(), args, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if result.Files != 2 || result.Fragments != 2 || result.TotalMatchGroups != 1 ||
		!result.Configuration.EmbeddedSQL || result.Configuration.SQLDialect != "postgresql" {
		t.Fatalf("embedded SQL report = %+v", result)
	}
	for _, coverage := range result.FileCoverage {
		if coverage.Language != "go" || coverage.ComparisonDomain != "sql-query" {
			t.Fatalf("embedded SQL coverage = %+v", coverage)
		}
	}
	for _, profile := range result.Groups[0].Profiles {
		for _, occurrence := range profile.Occurrences {
			if occurrence.Location.Language != "postgresql" ||
				occurrence.Location.ComparisonDomain != "sql-query" ||
				occurrence.Parent == nil || occurrence.Parent.Language != "go" {
				t.Fatalf("embedded occurrence = %+v", occurrence)
			}
		}
	}
	var repeated bytes.Buffer
	stderr.Reset()
	if code := Run(context.Background(), args, &repeated, &stderr); code != exitSuccess ||
		!bytes.Equal(stdout.Bytes(), repeated.Bytes()) {
		t.Fatalf("repeated embedded SQL scan = %d/%q; output changed", code, stderr.String())
	}
}

func TestRunScanStatementBlocksFindsPartialFunctionDuplication(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := `package sample
func first(value string) {
	clean := strings.TrimSpace(value)
	result := strings.ToLower(clean)
	fmt.Println(result)
	println("first only")
}
func second(input string) {
	println("second only")
	clean := strings.TrimSpace(input)
	result := strings.ToLower(clean)
	fmt.Println(result)
}
`
	if err := os.WriteFile(filepath.Join(root, "blocks.go"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--statement-blocks", "--block-statements", "3",
		"--min-tokens", "1", "--threshold", "1", root,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	foundBlock := false
	for _, group := range result.Groups {
		for _, profile := range group.Profiles {
			for _, occurrence := range profile.Occurrences {
				if occurrence.Location.FragmentKind == "block" {
					foundBlock = true
					if occurrence.Parent == nil {
						t.Fatalf("block occurrence lacks parent: %+v", occurrence)
					}
				}
			}
		}
	}
	if !foundBlock || !result.Configuration.StatementBlocks ||
		result.Configuration.BlockStatements != 3 || result.Configuration.MaxBlocksPerFunc != 64 {
		t.Fatalf("statement-block report = %+v", result)
	}
}

func TestRunLanguagesHelp(t *testing.T) {
	t.Parallel()

	for _, argument := range []string{"--help", "-h"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(context.Background(), []string{"languages", argument}, &stdout, &stderr)
		if code != exitSuccess || stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), "extensionless-script shebang interpreters") {
			t.Fatalf("languages %s = %d/%q/%q", argument, code, stdout.String(), stderr.String())
		}
	}
}

func TestRunScanHelpListsDialectAndMultiWorktreeFocus(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--help"}, &stdout, &stderr)
	if code != exitSuccess || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "-sql-dialect") ||
		!strings.Contains(stderr.String(), "generic or postgresql") ||
		!strings.Contains(stderr.String(), "-changed-worktree") ||
		!strings.Contains(stderr.String(), "PATH=REVISION") {
		t.Fatalf("scan help = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestRunScanSQLQueries(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--threshold", "0.70", "--min-tokens", "12",
		"../../examples/sql-queries",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.Files != 4 || result.Fragments != 4 || result.TotalMatchGroups != 1 ||
		len(result.Groups) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("SQL report summary = %+v", result)
	}
	profile := result.Groups[0].Profiles[0]
	if profile.OccurrenceCount != 2 || len(profile.Occurrences) != 2 {
		t.Fatalf("SQL profile = %+v", profile)
	}
	for _, occurrence := range profile.Occurrences {
		location := occurrence.Location
		if location.Language != "sql" || location.LanguageFamily != "sql" ||
			location.ComparisonDomain != "sql-query" || location.FragmentKind != "query" {
			t.Fatalf("SQL location = %+v", location)
		}
	}
}

func TestRunScanShellDialects(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--threshold", "0.85", "--min-tokens", "12",
		"--language-pair", "bash,zsh", "../../examples/shell-validation",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.Files != 3 || result.Fragments != 3 || result.TotalMatchGroups != 1 ||
		len(result.Groups) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("shell report summary = %+v", result)
	}
	languages := make(map[string]bool)
	locations := make([]model.Location, 0, 2)
	for _, profile := range result.Groups[0].Profiles {
		for _, occurrence := range profile.Occurrences {
			locations = append(locations, occurrence.Location)
		}
	}
	if len(locations) != 2 {
		t.Fatalf("shell locations = %+v", locations)
	}
	for _, location := range locations {
		languages[location.Language] = true
		if location.LanguageFamily != "shell" || location.ComparisonDomain != "code" ||
			location.FragmentKind != "function" {
			t.Fatalf("shell location = %+v", location)
		}
	}
	if !languages["bash"] || !languages["zsh"] {
		t.Fatalf("shell languages = %+v", languages)
	}
}

func TestRunScanSwiftAndGo(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--threshold", "0.80", "--min-tokens", "12",
		"--language-pair", "go,swift", "../../examples/swift-validation",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.Files != 3 || result.Fragments != 3 || result.TotalMatchGroups != 1 ||
		len(result.Groups) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("Swift report summary = %+v", result)
	}
	languages := make(map[string]bool)
	for _, profile := range result.Groups[0].Profiles {
		for _, occurrence := range profile.Occurrences {
			languages[occurrence.Location.Language] = true
		}
	}
	if !languages["go"] || !languages["swift"] {
		t.Fatalf("Swift group languages = %+v", languages)
	}
}

func TestRunFiltersDomainsBeforeParsingAndReportsSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"left.go":   "package sample\nfunc Left(value string) string { return value }\n",
		"right.go":  "package sample\nfunc Right(input string) string { return input }\n",
		"left.sql":  "SELECT id FROM users WHERE active = TRUE;\n",
		"right.sql": "SELECT id FROM members WHERE active = TRUE;\n",
		"schema.sql": "PRAGMA foreign_keys = ON;\n" +
			"CREATE TABLE documents (metadata TEXT CHECK(json_valid(metadata)));\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	args := []string{
		"scan", "--format", "json", "--threshold", "1", "--min-tokens", "1",
		"--comparison-domain", "code", root,
	}
	var first bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(context.Background(), args, &first, &stderr); code != exitSuccess {
		t.Fatalf("code scan exit = %d, stderr = %q", code, stderr.String())
	}
	var codeReport model.Report
	if err := json.Unmarshal(first.Bytes(), &codeReport); err != nil {
		t.Fatalf("decode code report: %v", err)
	}
	if codeReport.Files != 2 || codeReport.Fragments != 2 ||
		codeReport.TotalMatchGroups != 1 || len(codeReport.Warnings) != 0 ||
		codeReport.Configuration.ComparisonDomain != "code" {
		t.Fatalf("code report = %+v", codeReport)
	}

	var second bytes.Buffer
	stderr.Reset()
	if code := Run(context.Background(), args, &second, &stderr); code != exitSuccess {
		t.Fatalf("repeated code scan exit = %d, stderr = %q", code, stderr.String())
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("repeated domain-filtered JSON was not byte-identical")
	}

	var sqlOutput bytes.Buffer
	stderr.Reset()
	if code := Run(context.Background(), []string{
		"scan", "--format", "json", "--threshold", "1", "--min-tokens", "1",
		"--comparison-domain", "sql-query", root,
	}, &sqlOutput, &stderr); code != exitSuccess {
		t.Fatalf("SQL scan exit = %d, stderr = %q", code, stderr.String())
	}
	var sqlReport model.Report
	if err := json.Unmarshal(sqlOutput.Bytes(), &sqlReport); err != nil {
		t.Fatalf("decode SQL report: %v", err)
	}
	if sqlReport.Files != 3 || sqlReport.Fragments != 2 ||
		sqlReport.TotalMatchGroups != 1 || len(sqlReport.Warnings) != 1 ||
		sqlReport.Configuration.ComparisonDomain != "sql-query" {
		t.Fatalf("SQL report = %+v", sqlReport)
	}
}

func TestRunScanJSONAndFailOnMatch(t *testing.T) {
	t.Parallel()

	args := []string{
		"scan",
		"--threshold", "0.70",
		"--cross-language-only",
		"--format", "json",
		"--fail-on-match",
		"../../examples/email-validation",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitFindings, stderr.String())
	}

	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if len(result.Groups) == 0 {
		t.Fatal("JSON report contains no groups")
	}
	if result.Warnings == nil {
		t.Fatal("warnings must encode as an empty array, not null")
	}
	if result.SchemaVersion != model.SchemaVersion || result.Tool.Name != "mori" || result.Tool.Revision == "" ||
		result.Tool.NormalizationVersion != normalize.Version {
		t.Fatalf("report provenance = schema %d, tool %#v", result.SchemaVersion, result.Tool)
	}
}

func TestRunAppliesPriorityPathRule(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	auth := filepath.Join(root, "auth")
	if err := os.MkdirAll(auth, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "package auth\nfunc authorize(value string) string { return value }\n"
	for _, name := range []string{"file.go", "folder.go"} {
		if err := os.WriteFile(filepath.Join(auth, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--threshold", "1", "--min-tokens", "1", "--format", "json",
		"--ranking", "review", "--priority-path", "**/auth/**=25", root,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if len(result.Configuration.PriorityPaths) != 1 || len(result.Groups) != 1 ||
		result.Groups[0].ReviewPriority < 25 ||
		!slices.Contains(result.Groups[0].ReviewSignals, "priority-path:**/auth/**(+25)") {
		t.Fatalf("priority report = %#v", result)
	}
}

func TestRunScanFocusPathAndFocusedPolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	left := filepath.Join(root, "left.go")
	right := filepath.Join(root, "right.go")
	content := "package sample\nfunc Same(value string) string { if value == \"\" { return \"\" }; return value }\n"
	for _, file := range []string{left, right} {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--threshold", "1", "--min-tokens", "1", "--format", "json",
		"--focus-path", left, "--fail-on-focused-match", root,
	}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.Configuration.Focus == nil || result.Configuration.Focus.Mode != "explicit" ||
		result.Configuration.Focus.DiscoveredFocusFiles != 1 || result.TotalFocusedMatchGroups != 1 ||
		len(result.Groups) != 1 || !result.Groups[0].Focused || result.Groups[0].FocusedCount != 1 {
		t.Fatalf("focused report = %+v", result)
	}
}

func TestRunScanChangedSinceIncludesWorkingTree(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Mori Test")
	runTestGit(t, root, "config", "user.email", "mori@example.invalid")
	left := filepath.Join(root, "left.go")
	right := filepath.Join(root, "right.go")
	content := "package sample\nfunc Same(value string) string { if value == \"\" { return \"\" }; return value }\n"
	for _, file := range []string{left, right} {
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	runTestGit(t, root, "add", ".")
	runTestGit(t, root, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(left, []byte(content+"// changed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile change: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--threshold", "1", "--min-tokens", "1", "--format", "json",
		"--changed-since", base, "--fail-on-focused-match", root,
	}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	focus := result.Configuration.Focus
	if focus == nil || focus.Mode != "git" || focus.RequestedBase != base ||
		focus.BaseCommit != base || focus.MergeBase != base || len(focus.HeadCommit) != 40 ||
		!focus.WorkingTreeIncluded || !focus.UntrackedIncluded ||
		!reflect.DeepEqual(focus.ChangedPaths, []string{"left.go"}) ||
		len(focus.Worktrees) != 0 ||
		focus.DiscoveredFocusFiles != 1 || result.TotalFocusedMatchGroups != 1 {
		t.Fatalf("Git focus report = %+v", result)
	}
}

func TestRunScanChangedWorktreeResolvesNestedRepositoriesIndependently(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Mori Test")
	runTestGit(t, root, "config", "user.email", "mori@example.invalid")
	content := "package sample\nfunc Same(value string) string { if value == \"\" { return \"\" }; return value }\n"
	for _, name := range []string{"left.go", "right.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	runTestGit(t, root, "add", ".")
	runTestGit(t, root, "commit", "-m", "base")
	rootBase := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))

	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	runTestGit(t, nested, "init", "--initial-branch=main")
	runTestGit(t, nested, "config", "user.name", "Mori Test")
	runTestGit(t, nested, "config", "user.email", "mori@example.invalid")
	for _, name := range []string{"nested.go", "copy.go"} {
		if err := os.WriteFile(filepath.Join(nested, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile nested: %v", err)
		}
	}
	runTestGit(t, nested, "add", ".")
	runTestGit(t, nested, "commit", "-m", "base")
	nestedBase := strings.TrimSpace(runTestGit(t, nested, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "left.go"), []byte(content+"// parent change\n"), 0o600); err != nil {
		t.Fatalf("WriteFile parent change: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "nested.go"), []byte(content+"// nested change\n"), 0o600); err != nil {
		t.Fatalf("WriteFile nested change: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--threshold", "1", "--min-tokens", "1", "--format", "json",
		"--changed-since", rootBase,
		"--changed-worktree", nested + "=" + nestedBase,
		"--fail-on-focused-match", root,
	}, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	focus := result.Configuration.Focus
	if focus == nil || focus.Mode != "git-worktrees" || len(focus.Worktrees) != 2 ||
		focus.RequestedBase != "" || focus.BaseCommit != "" || focus.MergeBase != "" ||
		focus.HeadCommit != "" || focus.DiscoveredFocusFiles != 2 || result.TotalFocusedMatchGroups == 0 {
		t.Fatalf("multi-worktree focus = %+v", result)
	}
	bases := map[string]string{}
	changed := map[string][]string{}
	for _, worktree := range focus.Worktrees {
		bases[filepath.Base(filepath.FromSlash(worktree.Root))] = worktree.RequestedBase
		changed[filepath.Base(filepath.FromSlash(worktree.Root))] = worktree.ChangedPaths
		if len(worktree.BaseCommit) != 40 || len(worktree.MergeBase) != 40 || len(worktree.HeadCommit) != 40 ||
			!worktree.WorkingTreeIncluded || !worktree.UntrackedIncluded {
			t.Fatalf("worktree provenance = %+v", worktree)
		}
	}
	if bases[filepath.Base(root)] != rootBase || bases["nested"] != nestedBase ||
		!reflect.DeepEqual(changed[filepath.Base(root)], []string{"left.go", "nested"}) ||
		!reflect.DeepEqual(changed["nested"], []string{"nested.go"}) {
		t.Fatalf("worktree bases = %#v, changes = %#v", bases, changed)
	}

	stdout.Reset()
	stderr.Reset()
	explicitArgs := []string{
		"scan", "--threshold", "1", "--min-tokens", "1", "--format", "json",
		"--changed-worktree", root + "=" + rootBase,
		"--changed-worktree", nested + "=" + nestedBase,
		"--fail-on-focused-match", root,
	}
	code = Run(context.Background(), explicitArgs, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("explicit worktrees exit = %d, stderr = %q", code, stderr.String())
	}
	result = model.Report{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode explicit-worktree JSON: %v\n%s", err, stdout.String())
	}
	if result.Configuration.Focus == nil || result.Configuration.Focus.Mode != "git-worktrees" ||
		len(result.Configuration.Focus.Worktrees) != 2 ||
		result.Configuration.Focus.DiscoveredFocusFiles != 2 {
		t.Fatalf("explicit-worktree focus = %+v", result.Configuration.Focus)
	}
	firstOutput := append([]byte{}, stdout.Bytes()...)
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), explicitArgs, &stdout, &stderr); code != exitFindings ||
		!bytes.Equal(stdout.Bytes(), firstOutput) {
		t.Fatalf("repeated explicit-worktree scan = %d, deterministic = %t, stderr = %q",
			code, bytes.Equal(stdout.Bytes(), firstOutput), stderr.String())
	}
}

func TestFocusPolicyValidation(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"scan", "--fail-on-match", "--fail-on-focused-match", "--focus-path", "file.go", "."},
		{"scan", "--fail-on-focused-match", "."},
		{"baseline", "update", "--baseline", "accepted.json", "--focus-path", "file.go", "."},
		{"scan", "--changed-worktree", "missing-separator", "."},
		{"baseline", "prune", "--baseline", "accepted.json", "--changed-worktree", ".=HEAD", "."},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := Run(context.Background(), args, &stdout, &stderr); code != exitUsage {
			t.Errorf("Run(%v) exit = %d, stderr = %q", args, code, stderr.String())
		}
	}
}

func TestParseChangedWorktreeSpecsRejectsMalformedAndUnboundedInput(t *testing.T) {
	t.Parallel()

	for _, values := range [][]string{{"missing-separator"}, {"=HEAD"}, {"nested="}} {
		if _, err := parseChangedWorktreeSpecs(values); err == nil ||
			!strings.Contains(err.Error(), "expected PATH=REVISION") {
			t.Errorf("parseChangedWorktreeSpecs(%v) error = %v", values, err)
		}
	}
	values := make([]string, maxChangedWorktrees+1)
	for index := range values {
		values[index] = fmt.Sprintf("nested-%d=HEAD", index)
	}
	if _, err := parseChangedWorktreeSpecs(values); err == nil ||
		!strings.Contains(err.Error(), "worktree safety limit") {
		t.Fatalf("unbounded specs error = %v", err)
	}
}

func runTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestBaselineUpdateAndScanSuppression(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join(t.TempDir(), "mori-baseline.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"baseline", "update",
		"--baseline", baselinePath,
		"--accept-all",
		"--threshold", "0.70",
		"--cross-language-only",
		"../../examples/email-validation",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("baseline update exit = %d, stderr = %q", code, stderr.String())
	}
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline was not written: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"scan",
		"--baseline", baselinePath,
		"--threshold", "0.70",
		"--cross-language-only",
		"--fail-on-match",
		"--format", "json",
		"../../examples/email-validation",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("suppressed scan exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode suppressed JSON: %v\n%s", err, stdout.String())
	}
	if result.TotalMatchGroups != 0 || result.SuppressedLocationPairs == 0 || result.Truncated {
		t.Fatalf("suppressed report = %+v, want no findings and visible suppression", result)
	}
	if result.Configuration.BaselineStatus != "compatible" ||
		result.Configuration.BaselineDigest == "" ||
		result.Configuration.BaselineDigest != result.Configuration.ScanProfileDigest {
		t.Fatalf("baseline profile evidence = %+v", result.Configuration)
	}
}

func TestBaselinePruneCheckReportsStaleEntries(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join(t.TempDir(), "mori-baseline.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	updateArgs := []string{
		"baseline", "update",
		"--baseline", baselinePath,
		"--accept-all",
		"--threshold", "0.70",
		"--cross-language-only",
		"../../examples/email-validation",
	}
	if code := Run(context.Background(), updateArgs, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("baseline update exit = %d, stderr = %q", code, stderr.String())
	}
	before, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile before prune check: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code := Run(context.Background(), []string{
		"baseline", "prune", "--baseline", baselinePath, "--check",
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation/validator.js",
	}, &stdout, &stderr)
	if code != exitFindings || !strings.Contains(stdout.String(), "stale") {
		t.Fatalf("prune check exit/output = %d/%q, stderr = %q", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile after prune check: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("prune --check modified the baseline")
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"baseline", "prune", "--baseline", baselinePath,
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation/validator.js",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("prune exit = %d, stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"baseline", "prune", "--baseline", baselinePath, "--check",
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation/validator.js",
	}, &stdout, &stderr)
	if code != exitSuccess || !strings.Contains(stdout.String(), "baseline is current") {
		t.Fatalf("post-prune check exit/output = %d/%q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestScanRejectsMissingBaseline(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--baseline", filepath.Join(t.TempDir(), "missing.json"), ".",
	}, &stdout, &stderr)
	if code != exitError || !strings.Contains(stderr.String(), "load baseline") {
		t.Fatalf("missing baseline exit/stderr = %d/%q", code, stderr.String())
	}
}

func TestRunRejectsInvalidThreshold(t *testing.T) {
	t.Parallel()

	for _, threshold := range []string{"70", "NaN", "+Inf"} {
		threshold := threshold
		t.Run(threshold, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"scan", "--threshold", threshold, "."},
				&stdout,
				&stderr,
			)
			if code != exitUsage {
				t.Fatalf("exit = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr.String(), "--threshold") {
				t.Fatalf("stderr = %q, want threshold explanation", stderr.String())
			}
		})
	}
}

func TestRunRejectsInvalidCoveragePolicies(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"coverage above one":  {"--min-file-coverage", "1.1"},
		"coverage NaN":        {"--min-file-coverage", "NaN"},
		"zero files below -1": {"--max-zero-fragment-files", "-2"},
	} {
		name, args := name, args
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			arguments := append([]string{"scan"}, args...)
			arguments = append(arguments, ".")
			code := Run(context.Background(), arguments, &stdout, &stderr)
			if code != exitUsage || !strings.Contains(stderr.String(), "coverage") &&
				!strings.Contains(stderr.String(), "zero-fragment") {
				t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
			}
		})
	}
}

func TestRunRejectsIncompatibleLanguageDomains(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--language-pair", "sql,go", ".",
	}, &stdout, &stderr)
	if code != exitUsage || !strings.Contains(stderr.String(), "incompatible comparison domains") {
		t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
	}
}

func TestRunRejectsInvalidSelectionCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "same and cross",
			args: []string{"scan", "--same-language-only", "--cross-language-only", "."},
			want: "mutually exclusive",
		},
		{
			name: "same and pair",
			args: []string{"scan", "--same-language-only", "--language-pair", "go,go", "."},
			want: "mutually exclusive",
		},
		{
			name: "unknown domain",
			args: []string{"scan", "--comparison-domain", "unknown", "."},
			want: "unknown --comparison-domain",
		},
		{
			name: "unknown SQL dialect",
			args: []string{"scan", "--sql-dialect", "mysql", "."},
			want: "unknown --sql-dialect",
		},
		{
			name: "unknown ranking",
			args: []string{"scan", "--ranking", "magic", "."},
			want: "unknown --ranking",
		},
		{
			name: "embedded SQL without domain",
			args: []string{"scan", "--embedded-sql", "."},
			want: "requires --comparison-domain sql-query",
		},
		{
			name: "blocks in SQL domain",
			args: []string{"scan", "--statement-blocks", "--comparison-domain", "sql-query", "."},
			want: "cannot be used with --comparison-domain sql-query",
		},
		{
			name: "invalid block size",
			args: []string{"scan", "--block-statements", "1", "."},
			want: "--block-statements must be from 2 to 10",
		},
		{
			name: "pair outside domain",
			args: []string{
				"scan", "--comparison-domain", "sql-query", "--language-pair", "go,go", ".",
			},
			want: "unselected comparison domain code",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if code := Run(context.Background(), test.args, &stdout, &stderr); code != exitUsage ||
				!strings.Contains(stderr.String(), test.want) {
				t.Fatalf("exit/stderr = %d/%q, want %q", code, stderr.String(), test.want)
			}
		})
	}
}

func TestConfigLoadsBeforeCLIOverridesAndLanguagePairsExpand(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, ".mori.json")
	if err := os.WriteFile(configPath, []byte(`{
  "threshold": 0.5,
  "max_groups": 250,
  "comparison_domain": "sql-query",
  "sql_dialect": "postgresql",
	"ranking": "review",
  "language_pairs": ["go,typescript"],
  "exclude": ["generated/**"],
	"exclude_generated": true,
  "respect_ignore": true,
  "baseline": "accepted.json"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stderr bytes.Buffer
	options, _, code, ok := parseScanOptions("scan", []string{
		"--config", configPath,
		"--threshold", "0.9",
		"--comparison-domain", "CODE",
		"--sql-dialect", "generic",
		"--no-ignore",
	}, &stderr, "")
	if !ok || code != exitSuccess {
		t.Fatalf("parse = %t/%d, stderr = %q", ok, code, stderr.String())
	}
	if options.threshold != 0.9 || options.maxGroups != 250 || options.respectIgnore ||
		len(options.languagePairs) != 1 || options.comparisonDomain != "CODE" ||
		options.sqlDialect != "generic" || options.ranking != "review" ||
		len(options.excludes) != 1 || !options.excludeGenerated ||
		options.baselinePath != filepath.Join(root, "accepted.json") {
		t.Fatalf("options = %#v", options)
	}
	pairs, err := expandLanguagePairs(options.languagePairs)
	if err != nil {
		t.Fatalf("expandLanguagePairs: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("expanded pairs = %#v, want Go paired with TS and TSX", pairs)
	}
	domain, _, err := resolveComparisonDomain(options.comparisonDomain)
	if err != nil {
		t.Fatalf("resolveComparisonDomain: %v", err)
	}
	if domain != "code" {
		t.Fatalf("resolved domain = %q", domain)
	}
}

func TestRunClassifiesAndExcludesGeneratedSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"generated.go": "// Code generated by fixture. DO NOT EDIT.\npackage sample\nfunc generated() { println(1) }\n",
		"main.go":      "package sample\nfunc main() { println(1) }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--min-tokens", "1", "--exclude-generated", root,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("scan exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if result.Files != 1 || len(result.FileCoverage) != 2 || !result.Configuration.ExcludeGenerated {
		t.Fatalf("generated exclusion report = %#v", result)
	}
	if result.FileCoverage[0].Status != "excluded_generated" ||
		!result.FileCoverage[0].Generated || result.FileCoverage[1].Status != "analyzed" {
		t.Fatalf("file coverage = %#v", result.FileCoverage)
	}
}

func TestRunAppliesSameLanguageConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, ".mori.json"),
		[]byte(`{"threshold":1,"min_tokens":1,"same_language_only":true}`),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	for name, content := range map[string]string{
		"left.ts":   "export const check = (value: string) => { return value.trim(); };\n",
		"right.tsx": "export const check = (value: string) => { return value.trim(); };\n",
		"other.go":  "package sample\nfunc check(value string) string { return value }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{
		"scan", "--format", "json", "--config", filepath.Join(root, ".mori.json"), root,
	}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("scan exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !result.Configuration.SameLanguageOnly || result.CandidatePairs != 1 ||
		result.TotalMatchGroups != 1 {
		t.Fatalf("same-language report = %+v", result)
	}
}

func TestRunRejectsUnknownConfigField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".mori.json")
	if err := os.WriteFile(path, []byte(`{"future":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--config", path, "."}, &stdout, &stderr)
	if code != exitError || !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
	}
}

func TestRunRequireCoverageDistinguishesEmptyFilesAndFragments(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		content map[string]string
		want    string
	}{
		{
			name: "unsupported",
			content: map[string]string{
				"script.fish": "echo hello\n",
			},
			want: "no supported source files",
		},
		{
			name: "no fragments",
			content: map[string]string{
				"constants.go": "package sample\nconst value = 1\n",
			},
			want: "no comparison fragments",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for name, content := range test.content {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Run(context.Background(), []string{
				"scan", "--format", "json", "--require-coverage", root,
			}, &stdout, &stderr)
			if code != exitCoverage || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("exit/stderr = %d/%q, want coverage failure %q", code, stderr.String(), test.want)
			}
			var result model.Report
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode report: %v\n%s", err, stdout.String())
			}
			if len(result.Warnings) == 0 || result.Warnings[len(result.Warnings)-1].Kind != "coverage" {
				t.Fatalf("warnings = %#v", result.Warnings)
			}
		})
	}
}

func TestRunRequireCoverageCanBeConfiguredAndOverridden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, ".mori.json")
	if err := os.WriteFile(configPath, []byte(`{"require_coverage":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{
		"scan", "--config", configPath, "--require-coverage=false", root,
	}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
}

func TestRunEnforcesStrictFileCoveragePolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"productive.go": "package sample\nfunc Productive() { println(\"ok\") }\n",
		"empty-a.go":    "package sample\nconst A = 1\n",
		"empty-b.go":    "package sample\nconst B = 2\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--format", "json", "--min-tokens", "1",
		"--min-file-coverage", "1", "--max-zero-fragment-files", "0", root,
	}, &stdout, &stderr)
	if code != exitCoverage ||
		!strings.Contains(stderr.String(), "file coverage 1/3") ||
		!strings.Contains(stderr.String(), "2 zero-fragment file(s)") {
		t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if result.Coverage.SupportedFiles != 3 || result.Coverage.AnalyzedFiles != 3 ||
		result.Coverage.FragmentFiles != 1 || result.Coverage.ZeroFragmentFiles != 2 {
		t.Fatalf("coverage = %+v", result.Coverage)
	}
	for _, file := range result.FileCoverage {
		if file.FragmentCount == 0 && file.ZeroReason != "no_boundaries" {
			t.Fatalf("zero-fragment file = %+v", file)
		}
	}
	if result.Configuration.MinFileCoverage != 1 || result.Configuration.MaxZeroFiles != 0 {
		t.Fatalf("configuration = %+v", result.Configuration)
	}
}

func TestRunCoverageWarningsAndParseDiagnosticsFailIndependently(t *testing.T) {
	t.Parallel()

	t.Run("warning", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unsupported.fish")
		if err := os.WriteFile(path, []byte("echo hello\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(context.Background(), []string{
			"scan", "--format", "json", "--fail-on-warning", path,
		}, &stdout, &stderr)
		if code != exitCoverage || !strings.Contains(stderr.String(), "warning(s) were reported") {
			t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
		}
	})

	t.Run("parse diagnostic", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.go")
		if err := os.WriteFile(path, []byte("package sample\nfunc Broken( {\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(context.Background(), []string{
			"scan", "--format", "json", "--min-tokens", "1",
			"--fail-on-parse-diagnostic", path,
		}, &stdout, &stderr)
		if code != exitCoverage || !strings.Contains(stderr.String(), "parse diagnostic(s)") {
			t.Fatalf("exit/stderr = %d/%q", code, stderr.String())
		}
	})
}

func TestBaselineUpdateDoesNotBypassStrictCoveragePolicies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "constants.go"),
		[]byte("package sample\nconst Value = 1\n"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	baselinePath := filepath.Join(root, "baseline.json")
	const original = "unchanged\n"
	if err := os.WriteFile(baselinePath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile baseline: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"baseline", "update", "--baseline", baselinePath,
		"--min-file-coverage", "1", root,
	}, &stdout, &stderr)
	if code != exitCoverage {
		t.Fatalf("exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	content, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile baseline: %v", err)
	}
	if string(content) != original {
		t.Fatalf("baseline changed to %q", content)
	}
}

func TestBaselineUpdatePreviewsBeforeExplicitAllAcceptance(t *testing.T) {
	t.Parallel()

	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"baseline", "update", "--baseline", baselinePath,
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation",
	}
	if code := Run(context.Background(), args, &stdout, &stderr); code != exitSuccess ||
		!strings.Contains(stdout.String(), "baseline preview") ||
		!strings.Contains(stdout.String(), "no file written") {
		t.Fatalf("preview exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(baselinePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview baseline stat = %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	args = append(args[:4], append([]string{"--accept-all"}, args[4:]...)...)
	if code := Run(context.Background(), args, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("accept-all exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("accepted baseline stat = %v", err)
	}
}

func TestBaselineSelectiveAddEditRemoveAndProfileMismatch(t *testing.T) {
	t.Parallel()

	var scanOut bytes.Buffer
	var scanErr bytes.Buffer
	if code := Run(context.Background(), []string{
		"scan", "--no-config", "--format", "json", "--threshold", "0.70",
		"--cross-language-only", "../../examples/email-validation",
	}, &scanOut, &scanErr); code != exitSuccess {
		t.Fatalf("scan exit/stderr = %d/%q", code, scanErr.String())
	}
	var report model.Report
	if err := json.Unmarshal(scanOut.Bytes(), &report); err != nil || len(report.Groups) < 2 {
		t.Fatalf("scan report groups/error = %d/%v", len(report.Groups), err)
	}
	identity := report.Groups[0].ID
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	common := []string{
		"--no-config", "--baseline", baselinePath, "--threshold", "0.70",
		"--cross-language-only", "../../examples/email-validation",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	addArgs := append([]string{"baseline", "add", "--identity", identity,
		"--note", "reviewed", "--classification", "intentional"}, common...)
	if code := Run(context.Background(), addArgs, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("add exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	set, err := baseline.Load(baselinePath)
	if err != nil || len(set.Entries()) != 1 || !set.Has(identity) {
		t.Fatalf("selective set/error = %#v/%v", set.Entries(), err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{
		"baseline", "edit", "--baseline", baselinePath, "--identity", identity,
		"--note", "reviewed again", "--classification", "necessary-duplication",
	}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("edit exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	updateArgs := append([]string{"baseline", "update", "--accept-all"}, common...)
	if code := Run(context.Background(), updateArgs, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("update exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	set, err = baseline.Load(baselinePath)
	if err != nil {
		t.Fatalf("Load updated: %v", err)
	}
	foundMetadata := false
	for _, entry := range set.Entries() {
		if entry.ID == identity && entry.Note == "reviewed again" &&
			entry.Classification == "necessary-duplication" {
			foundMetadata = true
		}
	}
	if !foundMetadata {
		t.Fatalf("metadata not preserved: %#v", set.Entries())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{
		"scan", "--no-config", "--baseline", baselinePath, "--threshold", "0.71",
		"--cross-language-only", "../../examples/email-validation",
	}, &stdout, &stderr); code != exitError || !strings.Contains(stderr.String(), "profile") {
		t.Fatalf("mismatch exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{
		"baseline", "remove", "--baseline", baselinePath, "--identity", identity,
	}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("remove exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	set, err = baseline.Load(baselinePath)
	if err != nil || set.Has(identity) {
		t.Fatalf("removed set/error = %#v/%v", set.Entries(), err)
	}
}

func TestBaselineProfileDetectsIgnoreFileContentChange(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		"left.go":    "package sample\nfunc Left(value string) string { return value }\n",
		"right.go":   "package sample\nfunc Right(input string) string { return input }\n",
		".gitignore": "# reviewed ignore policy\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	var scanOut bytes.Buffer
	var stderr bytes.Buffer
	common := []string{
		"--no-config", "--threshold", "0.70", "--min-tokens", "1",
		"--same-language-only", root,
	}
	if code := Run(context.Background(), append(
		[]string{"scan", "--format", "json"}, common...,
	), &scanOut, &stderr); code != exitSuccess {
		t.Fatalf("scan exit/stderr = %d/%q", code, stderr.String())
	}
	var report model.Report
	if err := json.Unmarshal(scanOut.Bytes(), &report); err != nil || len(report.Groups) == 0 {
		t.Fatalf("scan report groups/error = %d/%v", len(report.Groups), err)
	}
	if len(report.Configuration.IgnoreEvidence) != 1 {
		t.Fatalf("ignore evidence = %#v", report.Configuration.IgnoreEvidence)
	}
	baselinePath := filepath.Join(root, "baseline.json")
	addArgs := append([]string{
		"baseline", "add", "--baseline", baselinePath,
		"--identity", report.Groups[0].ID,
	}, common...)
	var stdout bytes.Buffer
	stderr.Reset()
	if code := Run(context.Background(), addArgs, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("add exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gitignore"),
		[]byte("# changed ignore policy\n"),
		0o600,
	); err != nil {
		t.Fatalf("rewrite ignore file: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	scanArgs := append([]string{"scan", "--baseline", baselinePath}, common...)
	if code := Run(context.Background(), scanArgs, &stdout, &stderr); code != exitError ||
		!strings.Contains(stderr.String(), "differs from active profile") {
		t.Fatalf("changed ignore exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestBaselineMigrationIsExplicitAndWarningsBlockMutation(t *testing.T) {
	t.Parallel()

	legacyPath := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(legacyPath, []byte(`{
  "schema_version": 2,
  "mori_version": "0.20.0",
  "normalization_version": 12,
  "identity_scope": "content",
  "threshold": 0.7,
  "entries": []
}`), 0o600); err != nil {
		t.Fatalf("WriteFile legacy: %v", err)
	}
	baseArgs := []string{
		"baseline", "migrate", "--no-config", "--baseline", legacyPath,
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{
		"scan", "--no-config", "--format", "json", "--baseline", legacyPath,
		"--threshold", "0.70", "--cross-language-only",
		"../../examples/email-validation",
	}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("legacy scan exit/stderr = %d/%q", code, stderr.String())
	}
	var legacyReport model.Report
	if err := json.Unmarshal(stdout.Bytes(), &legacyReport); err != nil {
		t.Fatalf("decode legacy report: %v", err)
	}
	if legacyReport.Configuration.BaselineStatus != "legacy" ||
		len(legacyReport.Warnings) != 1 || legacyReport.Warnings[0].Kind != "baseline" {
		t.Fatalf("legacy profile evidence = %+v/%#v", legacyReport.Configuration, legacyReport.Warnings)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), baseArgs, &stdout, &stderr); code != exitUsage ||
		!strings.Contains(stderr.String(), "--accept-profile") {
		t.Fatalf("implicit migration exit/stderr = %d/%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	migrateArgs := append(baseArgs[:2], append([]string{"--accept-profile"}, baseArgs[2:]...)...)
	if code := Run(context.Background(), migrateArgs, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("migration exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	set, err := baseline.Load(legacyPath)
	if err != nil || set.Legacy() || set.ProfileDigest() == "" {
		t.Fatalf("migrated set/error = %#v/%v", set, err)
	}

	root := t.TempDir()
	for name, content := range map[string]string{
		"left.go":   "package sample\nfunc Left(value string) string { return value }\n",
		"right.go":  "package sample\nfunc Right(input string) string { return input }\n",
		"broken.go": "package sample\nfunc Broken( {\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	warningBaseline := filepath.Join(root, "baseline.json")
	warningArgs := []string{
		"baseline", "update", "--no-config", "--accept-all",
		"--baseline", warningBaseline, "--threshold", "0.70", "--min-tokens", "1",
		"--same-language-only", root,
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), warningArgs, &stdout, &stderr); code != exitCoverage ||
		!strings.Contains(stderr.String(), "disallowed parse warning") {
		t.Fatalf("warning refusal exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(warningBaseline); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("warning baseline stat = %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	allowedArgs := append(warningArgs[:4], append([]string{"--allow-warning", "parse"}, warningArgs[4:]...)...)
	if code := Run(context.Background(), allowedArgs, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("allowed warning exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestRunSkillHelp(t *testing.T) {
	t.Parallel()

	for _, argument := range []string{"help", "-h", "--help"} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := Run(context.Background(), []string{"skill", argument}, &stdout, &stderr); code != exitSuccess ||
			!strings.Contains(stdout.String(), "mori skill install") || stderr.Len() != 0 {
			t.Fatalf("skill %s exit/stdout/stderr = %d/%q/%q", argument, code, stdout.String(), stderr.String())
		}
	}
}

func TestRunAppliesConfigAndIgnoreFilesEndToEnd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, content := range map[string]string{
		".mori.json":                            `{"threshold":1,"min_tokens":1,"max_groups":10}`,
		".gitignore":                            "generated/\n",
		"left.go":                               "package sample\nfunc Left(value string) string { if value == \"\" { return \"\" }; return value }\n",
		"right.go":                              "package sample\nfunc Right(input string) string { if input == \"\" { return \"\" }; return input }\n",
		filepath.Join("generated", "broken.ts"): "export function broken(: {\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--config", filepath.Join(root, ".mori.json"), "--format", "json", root,
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if result.Files != 2 || result.TotalMatchGroups != 1 || len(result.Warnings) != 0 ||
		len(result.Configuration.IgnoreFiles) != 1 || result.Configuration.ConfigPath == "" {
		t.Fatalf("report = %+v", result)
	}
}

func TestRunSkillInstallTarget(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent},
		&stdout,
		&stderr,
	)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Installed mori-review-similarity") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(parent, "mori-review-similarity", "SKILL.md")); err != nil {
		t.Fatalf("Stat(installed SKILL.md): %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent},
		&stdout,
		&stderr,
	)
	if code != exitSuccess || !strings.Contains(stdout.String(), "is current") {
		t.Fatalf("current exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}

	skillPath := filepath.Join(parent, "mori-review-similarity", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("custom\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(custom): %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent},
		&stdout,
		&stderr,
	)
	if code != exitError || !strings.Contains(stderr.String(), "--replace") {
		t.Fatalf("different exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(
		context.Background(),
		[]string{"skill", "install", "--target", parent, "--replace"},
		&stdout,
		&stderr,
	)
	if code != exitSuccess || !strings.Contains(stdout.String(), "Previous copy:") {
		t.Fatalf("replace exit/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestRunSkillInstallProject(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"skill", "install", "--project", project},
		&stdout,
		&stderr,
	)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	installed := filepath.Join(
		project,
		".agents",
		"skills",
		"mori-review-similarity",
		"SKILL.md",
	)
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("Stat(project SKILL.md): %v", err)
	}
}

func TestRunSkillInstallGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(
		context.Background(),
		[]string{"skill", "install", "--global"},
		&stdout,
		&stderr,
	)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	installed := filepath.Join(
		home,
		".agents",
		"skills",
		"mori-review-similarity",
		"SKILL.md",
	)
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("Stat(global SKILL.md): %v", err)
	}
}

func TestRunSkillInstallRequiresOneScope(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"skill", "install"},
		{"skill", "install", "--global", "--target", t.TempDir()},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(context.Background(), args, &stdout, &stderr)
		if code != exitUsage {
			t.Errorf("Run(%v) exit = %d, stderr = %q", args, code, stderr.String())
		}
	}
}

func TestReadOnlyCommandsReportWriteFailures(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"help"}, {"languages"}, {"version"}} {
		if code := Run(
			context.Background(),
			args,
			failingWriter{},
			io.Discard,
		); code != exitError {
			t.Errorf("Run(%v) exit = %d, want %d", args, code, exitError)
		}
	}
	if code := Run(
		context.Background(),
		[]string{"skill", "--help"},
		failingWriter{},
		io.Discard,
	); code != exitError {
		t.Errorf("Run(skill --help) exit = %d, want %d", code, exitError)
	}
	if code := Run(
		context.Background(),
		[]string{"scan", "--help"},
		io.Discard,
		failingWriter{},
	); code != exitError {
		t.Errorf("Run(scan --help) exit = %d, want %d", code, exitError)
	}
	if code := Run(
		context.Background(),
		[]string{"baseline", "update", "--help"},
		io.Discard,
		failingWriter{},
	); code != exitError {
		t.Errorf("Run(baseline update --help) exit = %d, want %d", code, exitError)
	}
	if code := Run(
		context.Background(),
		[]string{"skill", "install", "--help"},
		io.Discard,
		failingWriter{},
	); code != exitError {
		t.Errorf("Run(skill install --help) exit = %d, want %d", code, exitError)
	}
}

func TestRunScanUsesBoundedStdinOverlayAndImplicitFocus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	disk := `package sample
func First(ok bool) { alpha(); if ok { beta() } }
func Second(values []int) int { total := 0; for _, value := range values { total += value }; return total }
`
	if err := os.WriteFile(path, []byte(disk), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	overlay := `package sample
func First(ok bool) { alpha(); if ok { beta() } }
func Second(flag bool) { logEvent(); if flag { storeRecord() } }
`
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{
		"scan", "--no-config", "--format", "json", "--same-language-only",
		"--threshold", "1", "--min-tokens", "1", "--stdin-path", path, root,
	}, strings.NewReader(overlay), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if result.SchemaVersion != model.SchemaVersion || result.Configuration.StdinPath == "" ||
		result.Configuration.Focus == nil || result.Configuration.Focus.DiscoveredFocusFiles != 1 ||
		len(result.Groups) != 1 || !result.Groups[0].Focused {
		t.Fatalf("overlay report = %#v", result)
	}
	if result.Groups[0].Profiles[0].Occurrences[0].Location.Name != "First" &&
		result.Groups[0].Profiles[1].Occurrences[0].Location.Name != "First" {
		t.Fatalf("overlay group = %#v", result.Groups[0])
	}

	if _, err := readStdinOverlay(context.Background(), strings.NewReader("12345"), 4); err == nil {
		t.Fatal("oversized stdin overlay succeeded")
	}
}

func TestRunScanStdinOverlayReselectsLegacyHack(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "legacy.php")
	if err := os.WriteFile(path, []byte("<?php\nfunction disk(): int { return 1; }\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	overlay := `<?hh // strict
function first(int $value): int { return $value + 1; }
function second(int $input): int { return $input + 1; }
`
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{
		"scan", "--no-config", "--format", "json", "--same-language-only",
		"--threshold", "1", "--min-tokens", "1", "--stdin-path", path, root,
	}, strings.NewReader(overlay), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if len(result.Groups) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("overlay report = %#v", result)
	}
	for _, profile := range result.Groups[0].Profiles {
		for _, occurrence := range profile.Occurrences {
			if occurrence.Location.Language != "hack" || occurrence.Location.LanguageFamily != "php-hack" {
				t.Fatalf("overlay location = %#v", occurrence.Location)
			}
		}
	}
}

func TestRunScanRejectsMissingOrBaselinedStdinPath(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		code int
	}{
		{args: []string{"scan", "--no-config", "--stdin-path", "missing.go", "."}, code: exitError},
		{args: []string{"scan", "--no-config", "--stdin-path", "file.go", "--baseline", "baseline.json", "."}, code: exitUsage},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := RunWithInput(
			context.Background(), test.args, strings.NewReader("package sample\n"), &stdout, &stderr,
		); code != test.code {
			t.Fatalf("RunWithInput(%v) exit = %d, stderr = %q", test.args, code, stderr.String())
		}
	}
}

func TestRunScanUsesNamedProjectScopeRootsAndOverrides(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, directory := range []string{"backend", "frontend"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		content := fmt.Sprintf("package sample\nfunc %sA(v int) int { return v + 1 }\nfunc %sB(x int) int { return x + 1 }\n", directory, directory)
		if err := os.WriteFile(filepath.Join(root, directory, "sample.go"), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	configPath := filepath.Join(root, ".mori.json")
	if err := os.WriteFile(configPath, []byte(`{
  "threshold": 0.7,
  "scopes": {"backend": {"roots": ["backend"], "threshold": 1, "min_tokens": 1, "same_language_only": true}}
}`), 0o600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"scan", "--config", configPath, "--scope", "backend", "--format", "json",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if result.Configuration.Scope != "backend" || !reflect.DeepEqual(result.Configuration.ScopeRoots, []string{"backend"}) ||
		result.Files != 1 || len(result.Groups) != 1 || result.Threshold != 1 {
		t.Fatalf("scope report = %#v", result)
	}
}

func TestRunScanStagedUsesIndexSourcesConfigAndIgnoreRules(t *testing.T) {
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	cliTestGit(t, root, "config", "user.name", "Mori Test")
	cliTestGit(t, root, "config", "user.email", "mori@example.invalid")
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	write(".mori.json", `{"threshold":1,"min_tokens":1,"same_language_only":true}`)
	write(".gitignore", "ignored.go\n")
	write("sample.go", "package sample\nfunc Base() int { return 0 }\n")
	write("ignored.go", "package sample\nfunc IgnoredA() int { return 1 }; func IgnoredB() int { return 1 }\n")
	cliTestGit(t, root, "add", ".mori.json", ".gitignore", "sample.go")
	cliTestGit(t, root, "add", "-f", "ignored.go")
	cliTestGit(t, root, "commit", "-m", "base")

	write("sample.go", "package sample\nfunc StagedA(v int) int { return v + 1 }\nfunc StagedB(x int) int { return x + 1 }\n")
	cliTestGit(t, root, "add", "sample.go")
	write("sample.go", "package sample\nfunc Working() int { return 99 }\n")
	write("ignored.go", "package sample\nfunc IgnoredA(v int) int { return v + 2 }\nfunc IgnoredB(x int) int { return x + 2 }\n")
	cliTestGit(t, root, "add", "-f", "ignored.go")
	write(".mori.json", `{"threshold":0.99,"min_tokens":999}`)
	write(".gitignore", "")

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--staged", "--format", "json", "--require-focused-coverage"}, &stdout, &stderr)
	if code != exitCoverage {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if result.Configuration.Input == nil || result.Configuration.Input.Mode != "git-index" ||
		result.Configuration.Input.WorkingTreeIncluded || result.Configuration.Input.UntrackedIncluded ||
		result.Configuration.Input.IndexDigest == "" || result.Configuration.Focus == nil ||
		result.Configuration.Focus.Mode != "git-index" || len(result.Groups) != 1 ||
		result.Configuration.Focus.CoveredFocusFiles != 1 || result.Configuration.Focus.RequiredFocusFiles != 2 ||
		result.Groups[0].Profiles[0].Occurrences[0].Location.Name == "Working" {
		t.Fatalf("staged report = %#v", result)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run(context.Background(), []string{
		"scan", "--staged", "--format", "json", "--include-focused", "--require-focused-coverage",
	}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("included exit = %d, stderr = %q", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode included report: %v", err)
	}
	if result.Configuration.Focus.CoveredFocusFiles != 2 || result.Configuration.Focus.RequiredFocusFiles != 2 {
		t.Fatalf("included focus = %#v", result.Configuration.Focus)
	}
}

func cliTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
