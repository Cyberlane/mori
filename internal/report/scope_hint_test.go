package report

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestScopeHintIsBoundedAndDoesNotChangeJSON(t *testing.T) {
	value := model.Report{}
	for i := 0; i < 30; i++ {
		path := "packages/runtime-test/src/main.ts"
		if i < 13 {
			path = "src/a.test.ts"
		}
		value.Groups = append(value.Groups, model.MatchGroup{Profiles: []model.FragmentProfile{{Occurrences: []model.FragmentSummary{{Location: model.Location{Path: path}}}}}})
	}
	for _, render := range []func(io.Writer, model.Report) error{Text, Compact, Agent} {
		var out bytes.Buffer
		if err := render(&out, value); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "13/25 leading retained groups") {
			t.Fatalf("missing bounded hint: %s", out.String())
		}
	}
	var out bytes.Buffer
	if err := JSON(&out, value); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "scope hint") {
		t.Fatal("human hint leaked into machine JSON")
	}
	value.Groups = value.Groups[13:]
	out.Reset()
	if err := scopeHint(&out, value); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("runtime-test falsely classified")
	}
	value.Groups = value.Groups[:4]
	out.Reset()
	if err := scopeHint(&out, value); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("tiny sample produced hint")
	}
}

func TestScopeHintPathBoundaries(t *testing.T) {
	for path, want := range map[string]bool{
		"src/__tests__/a.ts": true, "src/a.stories.tsx": true,
		"packages/runtime-test/src/main.ts": false, "src/test_helpers.ts": false,
		"/tmp/tests/project/src/a.ts": false, "../tests/project/src/a.ts": false,
		"src/[route]/new.test.ts": true,
	} {
		if got := conventionalReviewTestPath(path); got != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}
}
