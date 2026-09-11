package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the printed command from outside the project, including paths and
// custom scope names requiring quoting. A positional root would lose src-only
// scope selection and incorrectly scan tooling.ts.
func TestSetupPrintedScanUsesNamedRoots(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell execution; Windows quoting is checked separately")
	}
	for _, intent := range []string{"application", "library"} {
		t.Run(intent, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "project's $(printf expanded) space")
			if err := os.MkdirAll(filepath.Join(root, "src"), 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"src/main.ts", "tooling.ts"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("export function twice(x: number) { return x * 2; }\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			answer, _ := json.Marshal(setupAnswers{ReviewIntent: intent, ScopeName: "friend's review", Roots: []string{"src"}})
			var out, stderr bytes.Buffer
			if code := RunWithInput(context.Background(), []string{"setup", "--answers", "-", "--apply", root}, bytes.NewReader(answer), &out, &stderr); code != 0 {
				t.Fatalf("setup %d: %s", code, stderr.String())
			}
			if !strings.Contains(out.String(), "Named scopes are opt-in") {
				t.Fatalf("missing opt-in guidance: %s", out.String())
			}
			args := printedSetupScanArgs(t, out.String())
			args = append(args, "--format", "json", "--min-tokens", "1")
			out.Reset()
			stderr.Reset()
			if code := Run(context.Background(), args, &out, &stderr); code != 0 {
				t.Fatalf("printed command %v: %d %s", args, code, stderr.String())
			}
			if bytes.Contains(out.Bytes(), []byte("tooling.ts")) || !bytes.Contains(out.Bytes(), []byte("main.ts")) {
				t.Fatalf("printed command lost scope roots: %s", out.String())
			}
		})
	}
}

func printedSetupScanArgs(t *testing.T, output string) []string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "mori scan ") {
			continue
		}
		// Capture the actual shell argument vector without running a global Mori.
		raw, err := exec.Command("sh", "-c", "mori() { printf '%s\\0' \"$@\"; }; "+line).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	}
	t.Fatalf("missing next scan: %s", output)
	return nil
}

func TestSetupNextScanKeepAndShellQuoting(t *testing.T) {
	var out bytes.Buffer
	if err := writeSetupNextScan(&out, "/project path", "/project path/.mori.json", setupAnswers{ReviewIntent: "keep"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "--scope") || !strings.Contains(out.String(), " -- '/project path'") {
		t.Fatalf("base scan: %s", out.String())
	}
	if got := quoteSetupShellArgument("x'y $(echo z)", "windows"); got != "'x''y $(echo z)'" {
		t.Fatalf("PowerShell argument: %s", got)
	}
}

func TestSetupHelpDoesNotRequireAnswersVersion(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"setup", "--help"}, &out, &stderr); code != 0 {
		t.Fatalf("help %d", code)
	}
	if strings.Contains(stderr.String(), "versioned setup answers") {
		t.Fatal("answers have no version field")
	}
}
