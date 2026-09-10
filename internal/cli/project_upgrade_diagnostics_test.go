package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectWorkflowDiagnostics(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, content, want string }{
		{"release", "At the start of source-changing work, compare .mori-version with releases/latest before continuing.", "per-task latest-release"},
		{"wrapped release", "At the start of source-changing work, compare .mori-version with the latest\n official Mori GitHub release.", "per-task latest-release"},
		{"exhaustive", "Inspect every focused Mori group in both source locations.", "exhaustive focused review"},
		{"wrapped exhaustive", "Mori: Inspect both source locations for every\n focused group.", "exhaustive focused review"},
		{"partial", "mori scan .\necho 'Partially staged files are not supported'\nexit 1", "partial-staging rejection"},
		{"partial wrapped", "Mori: Partially staged files are\nnot supported.", "partial-staging rejection"},
		{"partial explicit reject", "Mori wrappers reject partially staged files.", "partial-staging rejection"},
		{"partial exit", "mori scan .\necho 'Partially staged files detected'\nexit 1", "partial-staging rejection"},
		{"task isolation", "Mori task readiness: Do not use the primary checkout's index or partially staged hunks as storage for another task.\n\nThe canonical staged-index contract requires complete coverage and rejects focused structural matches.", ""},
		{"unrelated same paragraph rejection", "Mori supports partially staged files. The gate rejects focused matches.", ""},
		{"unrelated exit", "Mori supports partially staged files.\nif ! command -v mori; then\n  exit 1\nfi", ""},
		{"current", "Trust the Mori pin. Do not query GitHub latest on each task. Deeply inspect at most 25 identities. mori review staged check .", ""},
		{"negation", "Do not inspect every focused Mori group. Never inspect both source locations for every focused group.", ""},
		{"unrelated", "Inspect every focused group in the image editor.", ""},
		{"partial rejection negated", "Mori: Do not reject partially staged files. Never reject partial staging.", ""},
		{"partial supported", "Mori supports partially staged files with mori review staged check .", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings, actions := projectAutomationPolicyDiagnostics(tc.content)
			if tc.want == "" {
				if len(findings) != 0 {
					t.Fatalf("unexpected findings: %v", findings)
				}
				return
			}
			if !strings.Contains(strings.Join(findings, "; "), tc.want) || len(actions) == 0 {
				t.Fatalf("findings/actions = %v/%v", findings, actions)
			}
		})
	}
}

func TestProjectWorkflowUpgradeLifecyclePreservesPolicy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	policy := []byte("# Mori\nAt the start of source-changing work, compare .mori-version with releases/latest before continuing.\nInspect every focused Mori group in both source locations.\n")
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, policy, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("apply %d: %s / %s", code, &stdout, &stderr)
	}
	if !strings.Contains(stdout.String(), "proposed policy:") {
		t.Fatalf("missing proposals: %s", &stdout)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, policy) {
		t.Fatal("apply changed project-owned policy")
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"project", "upgrade", "--check", root}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("advisory policy blocked compatibility: %d: %s / %s", code, &stdout, &stderr)
	}
	first := inspectProjectAutomation(root)
	second := inspectProjectAutomation(root)
	if first.Detail != second.Detail || first.Action != second.Action {
		t.Fatal("nondeterministic diagnostics")
	}
}

func TestProjectWorkflowInspectionBoundsAndSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), bytes.Repeat([]byte("x"), 1024*1024+1), 0600); err != nil {
		t.Fatal(err)
	}
	component := inspectProjectAutomation(root)
	if !strings.Contains(component.Detail, "inspection skipped") {
		t.Fatalf("missing size diagnostic: %+v", component)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "pre-commit"), []byte("mori scan .; echo 'partially staged files not supported'; exit 1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".githooks")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	component = inspectProjectAutomation(root)
	if strings.Contains(component.Detail, "partial-staging rejection") {
		t.Fatal("followed symlinked parent")
	}
}

func TestProjectWorkflowInspectionFileLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, ".githooks")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 129; i++ {
		content := "mori review staged check ."
		if i == 128 {
			content = "mori: Inspect every focused Mori group."
		}
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("%03d", i)), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	component := inspectProjectAutomation(root)
	if !strings.Contains(component.Detail, "inspection skipped") || strings.Contains(component.Detail, "exhaustive focused review") {
		t.Fatalf("file limit not respected: %+v", component)
	}
}
