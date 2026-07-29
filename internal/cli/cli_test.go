package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRunLanguages(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), []string{"languages"}, &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	for _, expected := range []string{"Go", "JavaScript / JSX", "Python", "Rust", "TypeScript / TSX"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("languages output missing %q", expected)
		}
	}
}

func TestRunScanJSONAndFailOnMatch(t *testing.T) {
	t.Parallel()

	args := []string{
		"scan",
		"--threshold", "0.70",
		"--cross-language-only",
		"--format", "json",
		"--fail-on-match",
		"../../examples/email-validation",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	if code != exitFindings {
		t.Fatalf("exit = %d, want %d; stderr = %q", code, exitFindings, stderr.String())
	}

	var result model.Report
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if len(result.Matches) == 0 {
		t.Fatal("JSON report contains no matches")
	}
	if result.Warnings == nil {
		t.Fatal("warnings must encode as an empty array, not null")
	}
}

func TestRunRejectsInvalidThreshold(t *testing.T) {
	t.Parallel()

	for _, threshold := range []string{"70", "NaN", "+Inf"} {
		threshold := threshold
		t.Run(threshold, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Run(
				context.Background(),
				[]string{"scan", "--threshold", threshold, "."},
				&stdout,
				&stderr,
			)
			if code != exitUsage {
				t.Fatalf("exit = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr.String(), "--threshold") {
				t.Fatalf("stderr = %q, want threshold explanation", stderr.String())
			}
		})
	}
}

func TestReadOnlyCommandsReportWriteFailures(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"help"}, {"languages"}, {"version"}} {
		if code := Run(
			context.Background(),
			args,
			failingWriter{},
			io.Discard,
		); code != exitError {
			t.Errorf("Run(%v) exit = %d, want %d", args, code, exitError)
		}
	}
	if code := Run(
		context.Background(),
		[]string{"scan", "--help"},
		io.Discard,
		failingWriter{},
	); code != exitError {
		t.Errorf("Run(scan --help) exit = %d, want %d", code, exitError)
	}
}
