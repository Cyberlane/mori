package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Cyberlane/mori/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestNamedApplicationScopeDoesNotNarrowCanonicalStagedGate(t *testing.T) {
	root := t.TempDir()
	cliTestGit(t, root, "init", "--initial-branch=main")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	for name, content := range map[string]string{".mori.json": `{"profile":"review","min_tokens":1,"scopes":{"application":{"roots":["."],"exclude":["**/*.test.ts"]}}}`, "main.ts": "export function twice(x: number) { return x * 2; }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cliTestGit(t, root, "add", ".")
	cliTestGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	if err := os.WriteFile(filepath.Join(root, "math.test.ts"), []byte("export function check(x: number) { return x * 2; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cliTestGit(t, root, "add", "math.test.ts")
	run := func(extra ...string) (int, model.Report) {
		t.Helper()
		var out, stderr bytes.Buffer
		args := []string{"review", "staged", "check", "--policy", "advisory", "--format", "json"}
		args = append(args, extra...)
		args = append(args, ".")
		code := Run(context.Background(), args, &out, &stderr)
		var result model.Report
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("%d %s %s", code, out.String(), stderr.String())
		}
		return code, result
	}
	code, inclusive := run()
	if code != 0 || inclusive.Configuration.Focus.CoveredFocusFiles != 1 || inclusive.Configuration.Focus.RequiredFocusFiles != 1 {
		t.Fatalf("inclusive %d %+v", code, inclusive.Configuration.Focus)
	}
	code, narrow := run("--scope", "application")
	if code != exitCoverage || narrow.Configuration.Focus.CoveredFocusFiles != 0 || narrow.Configuration.Focus.RequiredFocusFiles != 1 {
		t.Fatalf("narrow %d %+v", code, narrow.Configuration.Focus)
	}
	if inclusive.Configuration.Input.IndexDigest != narrow.Configuration.Input.IndexDigest {
		t.Fatal("scope changed immutable input")
	}
	if inclusive.Configuration.ScanProfileDigest == narrow.Configuration.ScanProfileDigest {
		t.Fatal("scope not bound to acceptance profile")
	}
}
