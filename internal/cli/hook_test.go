package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookPreCommitReceiptEnvironmentContract(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"config", "user.name", "Mori Test"}, {"config", "user.email", "mori@example.invalid"}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc Sample() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", root, "add", "sample.go")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, output)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	t.Setenv("MORI_STAGED_REVIEW_RECEIPT", "yes")
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"hook", "pre-commit", "--no-config", "."}, &stdout, &stderr); code != exitUsage || !strings.Contains(stderr.String(), "must be unset, empty, or exactly 1") {
		t.Fatalf("invalid receipt env exit/output = %d/%q", code, stderr.String())
	}

	t.Setenv("MORI_STAGED_REVIEW_RECEIPT", "")
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"hook", "pre-commit", "--no-config", "."}, &stdout, &stderr); code != exitSuccess {
		t.Fatalf("ordinary hook exit = %d, stderr = %q", code, stderr.String())
	}

	t.Setenv("MORI_STAGED_REVIEW_RECEIPT", "1")
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"hook", "pre-commit", "--no-config", "."}, &stdout, &stderr); code != exitError || !strings.Contains(stderr.String(), "load review receipt") {
		t.Fatalf("receipt hook exit/output = %d/%q", code, stderr.String())
	}
}

func TestHookPreCommitUsesCanonicalFocusedFindingPolicy(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"config", "user.name", "Mori Test"}, {"config", "user.email", "mori@example.invalid"}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	for name, function := range map[string]string{
		"left.go":  "func Left(value int) int { return value + 1 }\n",
		"right.go": "func Right(value int) int { return value + 2 }\n",
	} {
		content := "package sample\n" + function
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("git", "-C", root, "add", "left.go", "right.go")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, output)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	t.Setenv("MORI_STAGED_REVIEW_RECEIPT", "")
	var hookOut, hookErr bytes.Buffer
	hookCode := Run(context.Background(), []string{"hook", "pre-commit", "--no-config", "--min-tokens", "1", "--format", "json", "."}, &hookOut, &hookErr)
	var reviewOut, reviewErr bytes.Buffer
	reviewCode := Run(context.Background(), []string{"review", "staged", "check", "--no-config", "--min-tokens", "1", "--format", "json", "."}, &reviewOut, &reviewErr)
	if hookCode != exitFindings || reviewCode != hookCode || hookOut.String() != reviewOut.String() {
		t.Fatalf("hook/review mismatch: hook %d/%q/%q review %d/%q/%q", hookCode, hookOut.String(), hookErr.String(), reviewCode, reviewOut.String(), reviewErr.String())
	}
}
