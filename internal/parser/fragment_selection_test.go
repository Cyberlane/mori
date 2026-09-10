package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

func TestRustFragmentSelectionUsesPositiveEvidence(t *testing.T) {
	content := `fn production() { consume(); }
#[test]
fn unit() { consume(); }
#[tokio::test(flavor = "current_thread")]
async fn asynchronous() { consume(); }
#[cfg(all(test, not(loom)))]
mod tests {
 fn helper() { consume(); }
}
#[cfg(any(test, feature = "production"))]
fn shared() { consume(); }
#[cfg(not(test))]
fn release() { consume(); }
fn tests_in_name() { consume(); }
`
	path := filepath.Join(t.TempDir(), "lib.rs")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	spec, _ := language.Detect(path)
	for _, test := range []struct {
		selection                         string
		count, excludedTest, excludedProd int
	}{{"", 7, 0, 0}, {"all", 7, 0, 0}, {"production", 4, 3, 0}, {"tests", 3, 0, 4}} {
		t.Run(test.selection, func(t *testing.T) {
			fragments, warnings, coverage := FileWithCoverage(context.Background(), source.File{Path: path, DisplayPath: "src/lib.rs", Language: spec}, Options{MinTokens: 1, FragmentSelection: test.selection})
			if len(warnings) != 0 || len(fragments) != test.count || coverage.CandidateFragments != 7 || coverage.ExcludedTestFragments != test.excludedTest || coverage.ExcludedProductionFragments != test.excludedProd {
				t.Fatalf("fragments=%+v warnings=%+v coverage=%+v", fragments, warnings, coverage)
			}
		})
	}
}

func TestCfgRequiresTest(t *testing.T) {
	for _, test := range []struct {
		predicate string
		want      bool
	}{
		{"test", true}, {"all(test,not(loom))", true}, {"all(any(test,feature=\"production\"),unix)", false}, {"any(test,all(test,unix))", true}, {"any(test,feature=\"production\")", false}, {"not(test)", false}, {"all(feature=\"test\")", false}, {"any()", false}, {"all()", false},
	} {
		if got := cfgRequiresTest(test.predicate); got != test.want {
			t.Errorf("%s = %v", test.predicate, got)
		}
	}
}

func TestTestPathSelectionIsBounded(t *testing.T) {
	spec, _ := language.Detect("input.ts")
	for _, c := range []struct {
		path string
		want bool
	}{
		{"src/__tests__/helper.ts", true}, {"tests/helper.ts", true}, {"src/main.test.ts", true}, {"src/main.spec.ts", true}, {"src/runtime-test/helper.ts", false}, {"src/testHelpers.ts", false}, {"src/test.ts", false}, {"src/stories/main.ts", false}, {"/home/tests/project/src/main.ts", false}, {"../tests/project/src/main.ts", false},
	} {
		if got := isTestFragment(nil, nil, source.File{DisplayPath: c.path, Language: spec}); got != c.want {
			t.Errorf("%s: %v", c.path, got)
		}
	}
}

func TestRustMacroCoverageWarningSurvivesSelection(t *testing.T) {
	for _, c := range []struct {
		content string
		warn    bool
	}{
		{`cfg_feature! { fn hidden() { consume(); } }`, true},
		{`mod inner { cfg_feature! { fn hidden() { consume(); } } }`, true},
		{`fn visible() { println!("hello"); }`, false},
	} {
		path := filepath.Join(t.TempDir(), "lib.rs")
		if err := os.WriteFile(path, []byte(c.content), 0600); err != nil {
			t.Fatal(err)
		}
		spec, _ := language.Detect(path)
		for _, selection := range []string{"all", "production", "tests"} {
			_, warnings, _ := FileWithCoverage(context.Background(), source.File{Path: path, DisplayPath: "src/lib.rs", Language: spec}, Options{MinTokens: 1, FragmentSelection: selection})
			if (len(warnings) > 0) != c.warn {
				t.Fatalf("%s %s warnings=%+v", c.content, selection, warnings)
			}
			if c.warn && warnings[0].Kind != "coverage" {
				t.Fatalf("warnings=%+v", warnings)
			}
		}
	}
}

func TestCfgClassificationBudgetIsConservative(t *testing.T) {
	if cfgRequiresTest(strings.Repeat("all(", 65) + "test" + strings.Repeat(")", 65)) {
		t.Fatal("over-depth predicate classified")
	}
	if cfgRequiresTest("all(test," + strings.Repeat("x", 8192) + ")") {
		t.Fatal("oversized predicate classified")
	}
}
