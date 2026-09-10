package cli

import (
	"bytes"
	"os"
	"testing"
)

func TestFragmentSelectionConfigScopeAndCLI(t *testing.T) {
	argumentTestChdir(t, t.TempDir())
	content := `{"fragment_selection":"production","scopes":{"tests":{"roots":["."],"fragment_selection":"tests"}}}`
	if err := os.WriteFile(".mori.json", []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args []string
		want string
	}{{nil, "production"}, {[]string{"--scope=tests"}, "tests"}, {[]string{"--scope=tests", "--fragment-selection=all"}, "all"}, {[]string{"--no-config"}, ""}} {
		var stderr bytes.Buffer
		options, _, code, ok := parseScanOptions("scan", test.args, &stderr, "")
		if !ok || code != exitSuccess || options.fragmentSelection != test.want {
			t.Fatalf("%v: selection=%q code=%d stderr=%s", test.args, options.fragmentSelection, code, &stderr)
		}
		profile, err := baselineScanProfile(options, nil)
		if err != nil {
			t.Fatal(err)
		}
		if profile.FragmentSelection != test.want {
			t.Fatalf("profile selection=%q", profile.FragmentSelection)
		}
	}
	for _, args := range [][]string{{"--fragment-selection=invalid"}, {"--scope=tests", "--fragment-selection=invalid"}} {
		var stderr bytes.Buffer
		_, _, code, ok := parseScanOptions("scan", args, &stderr, "")
		if ok || code != exitUsage {
			t.Fatalf("invalid selection: code=%d ok=%t stderr=%s", code, ok, &stderr)
		}
	}
}
