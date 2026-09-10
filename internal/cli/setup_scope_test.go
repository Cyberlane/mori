package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/source"
	"github.com/bmatcuk/doublestar/v4"
)

func TestSetupScopesKeepStagedBaseInclusive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	paths := []string{"packages/runtime-test/src/main.ts", "packages/core/src/a.test.ts", "frontend/a.spec.tsx", "server/main.go", "server/main_test.go", "service/src/main/java/A.java", "service/src/test/java/ATest.java", ".specify/tool.py", "test.js", "lib/main.rs", "main.zig", "view.vue"}
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("// fixture\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"setup", "--agent", root}, &out, &stderr); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	var plan setupPlan
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	var app scopeSuggestion
	for _, s := range plan.Suggestions {
		if s.Intent == "application" {
			app = s
		}
	}
	if app.ExcludedFiles != 5 || app.IncludedFiles != 5 {
		t.Fatalf("suggestion %+v", app)
	}
	for _, p := range app.Excludes {
		if strings.Contains(p, "runtime-test") || strings.Contains(p, ".specify") {
			t.Fatalf("false exclusion %s", p)
		}
	}
	out.Reset()
	stderr.Reset()
	answers := `{"review_intent":"application"}`
	if code := RunWithInput(context.Background(), []string{"setup", "--answers", "-", "--apply", root}, strings.NewReader(answers), &out, &stderr); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	settings, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Excludes) != 0 || len(settings.Scopes["application"].Excludes) != 5 {
		t.Fatalf("base narrowed or scope missing %+v", settings)
	}
	// A second attempt must not silently overwrite a reviewed existing scope.
	out.Reset()
	stderr.Reset()
	if code := RunWithInput(context.Background(), []string{"configure", "--answers", "-", "--apply", root}, strings.NewReader(answers), &out, &stderr); code != exitUsage {
		t.Fatalf("overwrite %d %s", code, stderr.String())
	}
}

func TestConfigurePreservesUnrelatedPolicyAcrossProfileChange(t *testing.T) {
	t.Parallel()
	current := config.Settings{Profile: "review", Baseline: "accepted.json", Excludes: []string{"owned/**"}, Scopes: map[string]config.ScopeSettings{"custom": {Roots: []string{"src"}}}, Workers: pointer(3)}
	result, err := applySetupAnswers(current, true, setupAnswers{Profile: "explore"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Baseline != current.Baseline || len(result.Scopes) != 1 || len(result.Excludes) != 1 || *result.Workers != 3 {
		t.Fatalf("lost unrelated policy %+v", result)
	}
}

func TestSetupInteractiveUsesSameQuestionsAndBufferedConfirmation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var out, stderr bytes.Buffer
	// One answer per shared question, then apply; a single buffer must retain yes.
	answers := strings.Repeat("\n", len(setupQuestions(config.Settings{}, false))) + "yes\n"
	if code := RunWithInput(context.Background(), []string{"setup", root}, strings.NewReader(answers), &out, &stderr); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, config.FileName)); err != nil {
		t.Fatalf("confirmation lost: %v; %s", err, out.String())
	}
	for _, q := range setupQuestions(config.Settings{}, false) {
		if !strings.Contains(out.String(), q.Prompt) {
			t.Fatalf("missing prompt %s", q.ID)
		}
	}
}

func TestScopeSuggestionsEscapeLiteralRoutingPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	paths := []string{"src/[id].test.ts", "src/[id]/tests/a.ts", "src/{route}/a?.test.ts", "src/plain.ts"}
	files := []source.File{}
	for _, path := range paths {
		files = append(files, source.File{Path: filepath.Join(root, path)})
	}
	suggestions := suggestSetupScopes(root, files)
	for _, s := range suggestions {
		if s.Intent != "application" {
			continue
		}
		for _, path := range paths[:3] {
			matched := false
			for _, pattern := range s.Excludes {
				ok, err := doublestar.Match(pattern, path)
				if err != nil {
					t.Fatal(err)
				}
				matched = matched || ok
			}
			if !matched {
				t.Fatalf("literal %q not excluded by %v", path, s.Excludes)
			}
		}
	}
	plan := setupPlan{Suggestions: suggestions}
	if _, err := applySetupScope(config.Settings{}, setupAnswers{ReviewIntent: "tests"}, plan); err != nil {
		t.Fatal(err)
	}
}

func TestSetupApplicationExcludesFutureConventionalTests(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(path string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("export function value() { return 1 }\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("src/[id].test.ts")
	write("src/{route}/tests/first.ts")
	write("src/demo.stories.tsx")
	var out, stderr bytes.Buffer
	if code := RunWithInput(context.Background(), []string{"setup", "--answers", "-", "--apply", root}, strings.NewReader(`{"review_intent":"application"}`), &out, &stderr); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	settings, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"new-package/added.test.ts", "new-package/tests/added.ts", "src/added.stories.tsx", "packages/runtime-test/src/main.ts", "src/contest.ts", "src/specification.ts"} {
		write(path)
	}
	result := source.Discover([]string{root}, source.Options{Excludes: settings.Scopes["application"].Excludes})
	var got []string
	for _, file := range result.Files {
		path, _ := filepath.Rel(root, file.Path)
		got = append(got, filepath.ToSlash(path))
	}
	want := "packages/runtime-test/src/main.ts,src/contest.ts,src/specification.ts"
	if strings.Join(got, ",") != want {
		t.Fatalf("future test exclusion lost or production excluded: %v; warnings %v", got, result.Warnings)
	}
	if len(settings.Excludes) != 0 {
		t.Fatal("base gate unexpectedly narrowed")
	}
}

func TestSetupScopesBoundLargeTestInventoriesWithoutTruncating(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var files []source.File
	for i := 0; i < 140; i++ {
		files = append(files, source.File{Path: filepath.Join(root, fmt.Sprintf("src/case%d.test.ts", i))})
	}
	files = append(files, source.File{Path: filepath.Join(root, "src/main.ts")})
	var application, library bool
	for _, s := range suggestSetupScopes(root, files) {
		switch s.Intent {
		case "application":
			application = true
			if len(s.Excludes) != 1 || s.ExcludedFiles != 140 || s.IncludedFiles != 1 {
				t.Fatalf("application %+v", s)
			}
		case "tests":
			t.Fatal("oversized literal test roots must not be truncated")
		case "library":
			library = true
		}
	}
	if !application || !library {
		t.Fatal("bounded application/library proposals missing")
	}
}

func TestSetupLibraryScopeSeparatesDemosAndTooling(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	paths := []string{"packages/core/src/main.ts", "packages/runtime-test/src/main.ts", "packages/[route]/lib/main.ts", "packages/core/src/main.test.ts", "examples/demo/src/main.ts", "demo.ts", ".specify/tool.py", "scripts/tool.ts"}
	var files []source.File
	for _, path := range paths {
		files = append(files, source.File{Path: filepath.Join(root, path)})
	}
	plan := setupPlan{Suggestions: suggestSetupScopes(root, files)}
	settings, err := applySetupScope(config.Settings{}, setupAnswers{ReviewIntent: "library"}, plan)
	if err != nil {
		t.Fatal(err)
	}
	scope := settings.Scopes["library"]
	if strings.Join(scope.Roots, ",") != "packages/[route]/lib,packages/core/src,packages/runtime-test/src" {
		t.Fatalf("unexpected roots %v", scope.Roots)
	}
	for _, suggestion := range plan.Suggestions {
		if suggestion.Intent == "library" && (suggestion.IncludedFiles != 3 || suggestion.ExcludedFiles != 5) {
			t.Fatalf("wrong library coverage %+v", suggestion)
		}
	}
	if len(settings.Excludes) != 0 {
		t.Fatal("base gate unexpectedly narrowed")
	}
}

func TestSetupTestConventionsCoverFutureFilesWithoutBroadNameExclusions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ observed, future, nearby string }{
		{"src/old_test.go", "new/next_test.go", "new/test_helpers.go"},
		{"test_old.py", "new/test_next.py", "new/latest_helpers.py"},
		{"old.spec.ts", "new/next.spec.tsx", "new/specification.ts"},
		{"old.test.ts", "new/next.test.js", "new/testimony.ts"},
		{"old.stories.tsx", "new/next.stories.ts", "new/storiesHelper.ts"},
		{"test.js", "new/test.js", "new/testHelper.js"},
		{"src/__tests__/old.ts", "new/__tests__/next.ts", "packages/runtime-test/src/main.ts"},
		{"src/[id]/tests/old.ts", "new/tests/next.ts", "src/test_helpers/main.ts"},
	} {
		t.Run(tt.observed, func(t *testing.T) {
			pattern, literal := testPathConvention(tt.observed)
			if pattern == "" || literal == "" {
				t.Fatal("observed test not recognized")
			}
			for _, path := range []string{tt.observed, tt.future} {
				if ok, err := doublestar.Match(pattern, path); err != nil || !ok {
					t.Fatalf("%s does not cover %s: %v", pattern, path, err)
				}
			}
			if ok, err := doublestar.Match(pattern, tt.nearby); err != nil || ok {
				t.Fatalf("%s overmatches %s: %v", pattern, tt.nearby, err)
			}
		})
	}
}
