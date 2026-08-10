package reviewreceipt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/buildinfo"
)

func TestReceiptRoundTripAndExactValidation(t *testing.T) {
	t.Parallel()
	evidence := testEvidence()
	receipt, err := New(evidence)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(receipt.FocusedMatchIDs) != 2 || receipt.FocusedMatchIDs[0] != "aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbb" {
		t.Fatalf("sorted IDs = %#v", receipt.FocusedMatchIDs)
	}
	path := filepath.Join(t.TempDir(), "nested", "receipt.json")
	if err := Write(path, receipt); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Validate(loaded, evidence); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if Digest(loaded) == "" {
		t.Fatal("empty receipt digest")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestReceiptRejectsStaleEvidenceAndMalformedDocuments(t *testing.T) {
	t.Parallel()
	evidence := testEvidence()
	receipt, err := New(evidence)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stale := evidence
	stale.IndexDigest = strings.Repeat("d", 64)
	if err := Validate(receipt, stale); err == nil || !strings.Contains(err.Error(), "index digest") {
		t.Fatalf("stale error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"decision":"accept-focused-structural-matches","focused_match_ids":["zzzzzzzzzzzzzzzz:aaaaaaaaaaaaaaaa"]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "invalid focused match ID") {
		t.Fatalf("malformed Load error = %v", err)
	}
}

func TestReceiptRequiresFocusedFindings(t *testing.T) {
	t.Parallel()
	evidence := testEvidence()
	evidence.FocusedMatchIDs = nil
	if _, err := New(evidence); err == nil || !strings.Contains(err.Error(), "no focused") {
		t.Fatalf("New error = %v", err)
	}
}

func testEvidence() Evidence {
	return Evidence{
		Tool:                 buildinfo.Info{Version: "0.29.0"},
		NormalizationVersion: 12,
		HeadCommit:           strings.Repeat("a", 40),
		IndexDigest:          strings.Repeat("b", 64),
		ScanProfileDigest:    strings.Repeat("c", 64),
		FocusedMatchIDs: []string{
			"cccccccccccccccc:dddddddddddddddd",
			"aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbb",
		},
	}
}
