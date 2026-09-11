package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/support"
)

func TestDiagnosticsCapturesEarlyFailures(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		args        []string
		code        int
	}{{"unknown", "arguments", []string{"--no-config", "--private-sentinel-invalid"}, exitUsage}, {"invalid-policy", "validation", []string{"--no-config", "--threshold", "2"}, exitUsage}, {"config", "configuration", []string{"--config", "missing-private-sentinel.json"}, exitError}, {"config-parent", "configuration", []string{"--config", "missing-private-sentinel-dir/config.json"}, exitError}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			args := append([]string{"scan", "--diagnostics", path}, tc.args...)
			var out, stderr bytes.Buffer
			if code := Run(context.Background(), args, &out, &stderr); code != tc.code {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			s, err := support.ReadSession(path)
			if err != nil {
				t.Fatalf("session: %v stderr %s", err, stderr.String())
			}
			if s.Failure == nil || s.Failure.Stage != tc.stage || s.Settings != nil {
				t.Fatalf("early session %+v", s)
			}
			raw, _ := os.ReadFile(path)
			if bytes.Contains(raw, []byte("private-sentinel")) {
				t.Fatal("private arguments leaked")
			}
		})
	}
}

func TestDiagnosticsSuccessMultiRootPrivacyAndOptOut(t *testing.T) {
	root := t.TempDir()
	dirs := []string{filepath.Join(root, "private-sentinel-one"), filepath.Join(root, "private-sentinel-two")}
	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "private-sentinel.ts"), []byte("function private_sentinel(x: number) { return x * 2; }"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "session.json")
	args := []string{"scan", "--no-config", "--min-tokens", "1", "--format", "json"}
	args = append(args, dirs...)
	var ordinary, diagnosed, stderr bytes.Buffer
	if code := Run(context.Background(), args, &ordinary, &stderr); code != 0 {
		t.Fatalf("normal %d %s", code, stderr.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("opt-out unexpectedly wrote session")
	}
	args = append([]string{"scan", "--diagnostics", path}, args[1:]...)
	stderr.Reset()
	if code := Run(context.Background(), args, &diagnosed, &stderr); code != 0 {
		t.Fatalf("diagnostics %d %s", code, stderr.String())
	}
	if !bytes.Equal(ordinary.Bytes(), diagnosed.Bytes()) {
		t.Fatal("diagnostics changed report")
	}
	s, err := support.ReadSession(path)
	if err != nil {
		t.Fatalf("session %v: %s", err, stderr.String())
	}
	if s.Status != "success" || s.Settings.RootCount != 2 || s.Counts.Files != 2 || s.Counts.Fragments != 2 || len(s.Phases) != 4 {
		t.Fatalf("session %+v", s)
	}
	raw, _ := os.ReadFile(path)
	for _, sentinel := range []string{"private-sentinel", "private_sentinel", root} {
		if bytes.Contains(raw, []byte(sentinel)) {
			t.Fatalf("session leaked %s", sentinel)
		}
	}
	// A pre-existing destination is never replaced and does not change scan exit.
	stderr.Reset()
	diagnosed.Reset()
	if code := Run(context.Background(), args, &diagnosed, &stderr); code != 0 {
		t.Fatal(code)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) || !strings.Contains(stderr.String(), "diagnostic session not written") {
		t.Fatal("existing destination not protected")
	}
}

func TestDiagnosticsDoesNotInterpretFlagValuesAsConsent(t *testing.T) {
	var out, stderr bytes.Buffer
	code := Run(context.Background(), []string{"scan", "--no-config", "--exclude", "--diagnostics", "--threshold", "2"}, &out, &stderr)
	if code != exitUsage {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "diagnostic session not written") {
		t.Fatal("flag value treated as consent")
	}
}

func TestDiagnosticsCandidateLimitAndDestinationConflicts(t *testing.T) {
	root := t.TempDir()
	content := `function one(x: number) { if (x > 1) { return x * 2; } return x; }
function two(x: number) { if (x > 2) { return x + 3; } return x; }
function three(x: number) { if (x > 3) { return x - 4; } return x; }`
	input := filepath.Join(root, "source.ts")
	if err := os.WriteFile(input, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "session.json")
	var out, stderr bytes.Buffer
	args := []string{"scan", "--diagnostics", path, "--no-config", "--min-tokens", "1", "--max-pairs", "1", input}
	if code := Run(context.Background(), args, &out, &stderr); code != exitError {
		t.Fatalf("limit %d %s", code, stderr.String())
	}
	s, err := support.ReadSession(path)
	if err != nil {
		t.Fatalf("session %v %s", err, stderr.String())
	}
	if s.Failure == nil || s.Failure.Code != "resource-limit" || s.Counts.CandidatePairs != 1 {
		t.Fatalf("limit session %+v", s)
	}
	for _, target := range []string{input, filepath.Join(root, ".mori.json")} {
		capture := scanDiagnostics{path: target, paths: []string{input}}
		if capture.safeDestination() {
			t.Fatalf("accepted protected destination %s", target)
		}
	}
}

func TestDiagnosticsLeadingGroupsAreBoundedPathCounts(t *testing.T) {
	report := model.Report{SchemaVersion: model.SchemaVersion}
	for i := 0; i < 30; i++ {
		path := "packages/runtime-test/src/main.ts"
		if i < 13 || i >= 25 {
			path = "src/fixture.test.ts"
		}
		report.Groups = append(report.Groups, model.MatchGroup{Profiles: []model.FragmentProfile{{Occurrences: []model.FragmentSummary{{Location: model.Location{Path: path}}}}}})
	}
	capture := scanDiagnostics{started: time.Now(), result: &report}
	session := capture.session(exitSuccess)
	if session.Counts.LeadingGroups != 25 || session.Counts.LeadingTestStoryGroups != 13 {
		t.Fatalf("sample %+v", session.Counts)
	}
	for path, want := range map[string]string{"/tmp/tests/project/main.ts": "", "../tests/project/main.ts": "", "src/test_helpers.ts": "", "src/new.test.ts": "test", "src/demo.stories.tsx": "story"} {
		if got := diagnosticPathClass(path); got != want {
			t.Fatalf("%s: %q want %q", path, got, want)
		}
	}
}

func TestDiagnosticsDiscoveryFailureHasNoReport(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.json")
	var out, stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{"scan", "--diagnostics", path, "--no-config", "--stdin-path", filepath.Join(root, "missing-private-sentinel.ts"), root}, strings.NewReader("function omitted(x) { return x; }"), &out, &stderr)
	if code != exitError {
		t.Fatalf("%d %s", code, stderr.String())
	}
	session, err := support.ReadSession(path)
	if err != nil {
		t.Fatalf("%v %s", err, stderr.String())
	}
	if session.ReportAvailable || session.Settings == nil || session.Failure == nil || session.Failure.Stage != "discovery" {
		t.Fatalf("session %+v", session)
	}
}

func TestDiagnosticsCancellationKeepsExitAndClassifiesOutcome(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	code := Run(ctx, []string{"scan", "--diagnostics", path, "--no-config", root}, &out, &stderr)
	if code != exitError {
		t.Fatalf("%d %s", code, stderr.String())
	}
	session, err := support.ReadSession(path)
	if err != nil {
		t.Fatalf("%v %s", err, stderr.String())
	}
	if session.Status != "cancelled" || session.Failure == nil || session.Failure.Code != "cancelled" {
		t.Fatalf("session %+v", session)
	}
}

func TestDiagnosticsDoesNotChangeAnalysisCacheIdentity(t *testing.T) {
	options := defaultScanOptions()
	before, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	options.diagnosticsPath = "private-session.json"
	options.diagnostics = &scanDiagnostics{started: time.Now()}
	after, err := stagedAnalysisCacheKey([]string{"."}, options, "", nil, "binary")
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("diagnostics invalidated analysis cache")
	}
	// A cached report has no current-run parser/comparison timings to report.
	options.diagnostics.result = &model.Report{SchemaVersion: model.SchemaVersion}
	session := options.diagnostics.session(exitSuccess)
	if len(session.Phases) != 1 || session.Phases[0].Name != "total" {
		t.Fatalf("invented cached phases %+v", session.Phases)
	}
}

func TestDiagnosticsHelpHasNoSideEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"scan", "--diagnostics", path, "--help"}, &out, &stderr); code != exitSuccess {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("help created diagnostics")
	}
}

func TestDiagnosticsPreservesLargestAcceptedWorkerSetting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.json")
	source := filepath.Join(root, "one.go")
	if err := os.WriteFile(source, []byte("package sample\nfunc twice(x int) int { return x * 2 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workers := int64(^uint(0) >> 1)
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"scan", "--diagnostics", path, "--no-config", "--min-tokens=1", "--workers", strconv.FormatInt(workers, 10), source}, &out, &stderr); code != exitSuccess {
		t.Fatalf("%d %s", code, stderr.String())
	}
	session, err := support.ReadSession(path)
	if err != nil {
		t.Fatalf("%v %s", err, stderr.String())
	}
	if session.Settings == nil || session.Settings.Workers != workers || session.Counts.Files != 1 {
		t.Fatalf("lost accepted worker setting %+v", session)
	}
}

func TestDiagnosticsOversizedOverlayIsDiscoveryFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.json")
	var out, stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{"scan", "--diagnostics", path, "--no-config", "--max-file-bytes=1", "--stdin-path", filepath.Join(root, "one.go"), root}, strings.NewReader("package sample"), &out, &stderr)
	if code != exitError {
		t.Fatalf("%d %s", code, stderr.String())
	}
	session, err := support.ReadSession(path)
	if err != nil {
		t.Fatalf("%v %s", err, stderr.String())
	}
	if session.Failure == nil || session.Failure.Stage != "discovery" || session.Failure.Code != "io" {
		t.Fatalf("misclassified overlay failure %+v", session)
	}
}

func TestDiagnosticsProtectsArtifactsThroughParentAliases(t *testing.T) {
	for _, kind := range []string{"config", "report", "input"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			real := filepath.Join(root, "real")
			alias := filepath.Join(root, "alias")
			if err := os.Mkdir(real, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, alias); err != nil {
				t.Skipf("directory symlinks unavailable: %v", err)
			}
			name := "protected.json"
			if kind == "input" {
				name = "protected.ts"
			}
			realPath := filepath.Join(real, name)
			aliasPath := filepath.Join(alias, name)
			args := []string{"scan", "--diagnostics", aliasPath}
			wantCode := exitSuccess
			switch kind {
			case "config":
				args = append(args, "--config", realPath)
				wantCode = exitError
			case "report":
				args = append(args, "--no-config", "--format", "agent", "--output", realPath, real)
			case "input":
				args = []string{"scan", "--diagnostics", realPath, "--no-config", aliasPath}
			}
			var out, stderr bytes.Buffer
			if code := Run(context.Background(), args, &out, &stderr); code != wantCode {
				t.Fatalf("%d %s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "destination conflicts with scan inputs or artifacts") {
				t.Fatalf("alias conflict missed: %s", stderr.String())
			}
			if kind == "report" {
				raw, err := os.ReadFile(realPath)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(raw, []byte("schema_version")) || bytes.Contains(raw, []byte("report_available")) {
					t.Fatalf("report replaced: %s", raw)
				}
			} else if _, err := os.Stat(realPath); !os.IsNotExist(err) {
				t.Fatalf("protected path created: %v", err)
			}
		})
	}
}

func TestDiagnosticsProtectsDanglingConfigTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "session.json")
	configLink := filepath.Join(root, "config-link.json")
	if err := os.Symlink(target, configLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"scan", "--diagnostics", target, "--config", configLink}, &out, &stderr); code != exitError {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("created dangling config target: %v", err)
	}
	if !strings.Contains(stderr.String(), "destination conflicts") {
		t.Fatalf("missed dangling conflict: %s", stderr.String())
	}
}
