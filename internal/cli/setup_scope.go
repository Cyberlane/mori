package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/config"
	"github.com/Cyberlane/mori/internal/source"
)

// Suggestions use visible path conventions, not an assertion of code ownership.
// They create named exploratory surfaces; base and canonical staged policy stay inclusive.
type scopeSuggestion struct {
	Intent        string   `json:"intent"`
	Roots         []string `json:"roots"`
	Excludes      []string `json:"exclude"`
	IncludedFiles int      `json:"included_files"`
	ExcludedFiles int      `json:"excluded_files"`
	Examples      []string `json:"examples"`
	Description   string   `json:"description"`
}

// Test exclusions generalize only conventions actually observed in the inventory.
// Test scope roots remain literal paths, independently of these exclusion globs.
func testPathConvention(path string) (pattern, literalRoot string) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts[:len(parts)-1] {
		switch part {
		case "test", "tests", "__tests__", "spec", "specs", "stories", "__stories__":
			return "**/" + part + "/**", strings.Join(parts[:i+1], "/")
		}
	}
	name := parts[len(parts)-1]
	switch {
	case strings.HasSuffix(name, "_test.go"):
		return "**/*_test.go", path
	case strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py"):
		return "**/test_*.py", path
	case strings.Contains(name, ".test."):
		return "**/*.test.*", path
	case strings.Contains(name, ".spec."):
		return "**/*.spec.*", path
	case strings.Contains(name, ".stories."):
		return "**/*.stories.*", path
	case name == "test.js":
		return "**/test.js", path
	}
	return "", ""
}

// Restrict automatic library proposals to recognizable source layouts. In
// particular, examples/demo src directories and repository tooling stay out.
func librarySourceRoot(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && (parts[0] == "src" || parts[0] == "lib") {
		return parts[0]
	}
	if len(parts) >= 4 && parts[0] == "packages" && (parts[2] == "src" || parts[2] == "lib") {
		return strings.Join(parts[:3], "/")
	}
	return ""
}

func suggestSetupScopes(root string, files []source.File) []scopeSuggestion {
	var tests, production, library []string
	patterns, literalTests, libraryRoots := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, file := range files {
		path, err := filepath.Rel(root, file.Path)
		if err != nil || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			continue
		}
		path = filepath.ToSlash(path)
		if pattern, literalRoot := testPathConvention(path); pattern != "" {
			tests = append(tests, path)
			patterns[pattern] = true
			literalTests[literalRoot] = true
		} else {
			production = append(production, path)
			if sourceRoot := librarySourceRoot(path); sourceRoot != "" {
				library = append(library, path)
				libraryRoots[sourceRoot] = true
			}
		}
	}
	sort.Strings(tests)
	sort.Strings(production)
	sort.Strings(library)
	keys := func(values map[string]bool) []string {
		result := make([]string, 0, len(values))
		for value := range values {
			result = append(result, value)
		}
		sort.Strings(result)
		return result
	}
	excludes, testRoots, libRoots := keys(patterns), keys(literalTests), keys(libraryRoots)
	sample := func(paths []string) []string { return append([]string{}, paths[:min(5, len(paths))]...) }
	result := []scopeSuggestion{{Intent: "all", Roots: []string{"."}, Excludes: []string{}, IncludedFiles: len(files), Examples: sample(append(append([]string{}, production...), tests...)), Description: "Explore all supported source under existing discovery rules; ignored and unsupported source remains outside this inventory."}}
	result = append(result, scopeSuggestion{Intent: "application", Roots: []string{"."}, Excludes: excludes, IncludedFiles: len(production), ExcludedFiles: len(tests), Examples: sample(production), Description: "Exclude observed test/story naming conventions, including future matching paths. Review these globs; inline tests require fragment selection and demos/tooling may remain."})
	// Never silently truncate literal roots. The application policy is bounded
	// by the small fixed set of conventions, even with thousands of tests.
	if len(testRoots) > 0 && len(testRoots) <= 128 {
		result = append(result, scopeSuggestion{Intent: "tests", Roots: testRoots, Excludes: []string{}, IncludedFiles: len(tests), ExcludedFiles: len(production), Examples: sample(tests), Description: "Review currently observed literal test/story paths. New colocated files require updating these roots; inline tests are not discovered."})
	}
	if len(libRoots) > 0 && len(libRoots) <= 128 {
		result = append(result, scopeSuggestion{Intent: "library", Roots: libRoots, Excludes: append([]string{}, excludes...), IncludedFiles: len(library), ExcludedFiles: len(files) - len(library), Examples: sample(library), Description: "Review observed src/lib or packages/<name>/src|lib roots, excluding observed test/story conventions. Demos and tooling outside these roots are omitted. Verify custom layouts and update roots when adding packages."})
	}
	if len(testRoots) > 128 {
		result[0].Description += " The tests suggestion is unavailable because more than 128 literal test roots were observed; configure it manually."
	}
	if len(libRoots) > 128 {
		result[0].Description += " The library suggestion is unavailable because more than 128 source roots were observed; configure it manually."
	}
	return result
}

func applySetupScope(settings config.Settings, answers setupAnswers, plan setupPlan) (config.Settings, error) {
	if answers.ReviewIntent == "" || answers.ReviewIntent == "keep" {
		if answers.Roots != nil || answers.ScopeName != "" || answers.ScopeExcludes != nil {
			return settings, errors.New("scope_name, roots and scope_exclude require review_intent")
		}
		return settings, nil
	}
	var proposal *scopeSuggestion
	for i := range plan.Suggestions {
		if plan.Suggestions[i].Intent == answers.ReviewIntent {
			proposal = &plan.Suggestions[i]
			break
		}
	}
	if proposal == nil {
		return settings, fmt.Errorf("no bounded %q scope suggestion is available; inspect the inventory and configure a named scope manually", answers.ReviewIntent)
	}
	name := answers.ScopeName
	if name == "" {
		name = answers.ReviewIntent
	}
	if settings.Scopes == nil {
		settings.Scopes = map[string]config.ScopeSettings{}
	}
	if _, exists := settings.Scopes[name]; exists {
		return settings, fmt.Errorf("scope %q already exists; choose another scope_name or edit that reviewed scope explicitly", name)
	}
	roots := proposal.Roots
	if answers.Roots != nil {
		roots = answers.Roots
	}
	if len(roots) == 0 {
		return settings, errors.New("scope roots must not be empty")
	}
	for _, root := range roots {
		clean := filepath.Clean(root)
		if root == "" || filepath.IsAbs(root) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return settings, fmt.Errorf("scope root %q must be a literal project-relative path", root)
		}
	}
	excludes := proposal.Excludes
	if answers.ScopeExcludes != nil {
		excludes = answers.ScopeExcludes
	}
	if err := source.ValidatePatterns(excludes); err != nil {
		return settings, err
	}
	settings.Scopes[name] = config.ScopeSettings{Roots: append([]string{}, roots...), Excludes: append([]string{}, excludes...)}
	return settings, nil
}

func showSetupInventory(w io.Writer, plan setupPlan) error {
	if _, err := fmt.Fprintf(w, "Selected inventory: %d supported file(s)\n", plan.Inventory.SupportedFiles); err != nil {
		return err
	}
	for _, item := range plan.Inventory.Languages {
		if _, err := fmt.Fprintf(w, "  %s: %d\n", item.ID, item.Files); err != nil {
			return err
		}
	}
	for _, item := range plan.Inventory.UnsupportedExtensions {
		if _, err := fmt.Fprintf(w, "  Not examined: %q (%d files)\n", item.Extension, item.Files); err != nil {
			return err
		}
	}
	for _, item := range plan.Inventory.Warnings {
		if _, err := fmt.Fprintf(w, "  Warning: %q\n", item); err != nil {
			return err
		}
	}
	for _, item := range plan.Suggestions {
		content, _ := json.Marshal(item)
		if _, err := fmt.Fprintf(w, "Suggested %s scope: %s\n", item.Intent, content); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "Suggestions are editable path conventions, not ownership or complete repository coverage. Named scopes do not narrow the base staged gate. Existing base excludes still apply to all scopes. Literal tests/library proposals exceeding 128 roots are omitted, never truncated; configure those scopes manually. Test exclusion globs cover only conventions observed during setup.")
	return err
}

func promptSetupPlan(reader *bufio.Reader, w io.Writer, plan setupPlan) (setupAnswers, error) {
	if err := showSetupInventory(w, plan); err != nil {
		return setupAnswers{}, err
	}
	values := map[string]any{}
	for _, q := range plan.Questions {
		if len(q.Choices) > 0 {
			def, _ := q.Default.(string)
			value, err := promptChoice(reader, w, q.Prompt, def, q.Choices)
			if err != nil {
				return setupAnswers{}, err
			}
			values[q.ID] = value
		} else {
			def, _ := json.Marshal(q.Default)
			if _, err := fmt.Fprintf(w, "%s [%s] (JSON): ", q.Prompt, def); err != nil {
				return setupAnswers{}, err
			}
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return setupAnswers{}, err
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var value any
			if err := json.Unmarshal([]byte(line), &value); err != nil {
				return setupAnswers{}, fmt.Errorf("%s: %w", q.ID, err)
			}
			values[q.ID] = value
		}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return setupAnswers{}, err
	}
	var result setupAnswers
	err = json.Unmarshal(raw, &result)
	return result, err
}
