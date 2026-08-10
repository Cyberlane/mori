package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/model"
)

func TestLSPInitializeAndShutdown(t *testing.T) {
	t.Parallel()
	input := lspTestFrame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///tmp/project"}}`) +
		lspTestFrame(`{"jsonrpc":"2.0","id":2,"method":"shutdown","params":null}`) +
		lspTestFrame(`{"jsonrpc":"2.0","method":"exit","params":null}`)
	var stdout, stderr bytes.Buffer
	code := RunWithInput(context.Background(), []string{"lsp"}, strings.NewReader(input), &stdout, &stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	reader := bufio.NewReader(&stdout)
	for _, wantID := range []float64{1, 2} {
		content, err := readLSPFrame(reader)
		if err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		if err := json.Unmarshal(content, &response); err != nil {
			t.Fatal(err)
		}
		if response["id"] != wantID {
			t.Fatalf("response = %#v", response)
		}
		if _, ok := response["result"]; !ok {
			t.Fatalf("response missing result: %#v", response)
		}
	}
}

func TestReadLSPFrameRejectsOversize(t *testing.T) {
	t.Parallel()
	input := fmt.Sprintf("Content-Length: %d\r\n\r\n", maxLSPMessageBytes+1)
	if _, err := readLSPFrame(bufio.NewReader(strings.NewReader(input))); err == nil {
		t.Fatal("oversize frame succeeded")
	}
}

func TestLSPDiagnosticsKeepSimilarityAdvisory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	documentPath := filepath.Join(root, "a.go")
	report := model.Report{Groups: []model.MatchGroup{{
		ID: "aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbb", Similarity: 0.91,
		Profiles: []model.FragmentProfile{{
			Occurrences: []model.FragmentSummary{
				{Location: model.Location{Path: documentPath, Language: "go", StartLine: 2, EndLine: 4}},
				{Location: model.Location{Path: filepath.Join(root, "b.go"), Language: "go", StartLine: 6, EndLine: 8}},
			},
		}},
	}}}
	diagnostics := diagnosticsForLSPDocument(report, documentPath, root)
	if len(diagnostics) != 1 || diagnostics[0].Code != "MORI001" || diagnostics[0].Severity != 3 || len(diagnostics[0].Related) != 1 || !strings.Contains(diagnostics[0].Message, "review both locations") {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func lspTestFrame(content string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(content), content)
}
