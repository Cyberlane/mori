package reviewreceipt

import (
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
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

func TestReceiptExplainsStagedReviewContractDrift(t *testing.T) {
	t.Parallel()
	evidence := testEvidence()
	receipt, err := New(evidence)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	active := evidence
	active.StagedReviewContract = StagedReviewContract{
		IncludeFocused:       false,
		RequireFocusCoverage: false,
		RequiredFocusedFiles: 24,
		CoveredFocusedFiles:  16,
	}
	err = Validate(receipt, active)
	if err == nil || !strings.Contains(err.Error(), "include_focused: receipt=true, active=false") ||
		!strings.Contains(err.Error(), "require_focused_coverage: receipt=true, active=false") ||
		!strings.Contains(err.Error(), "focused coverage: receipt=24/24, active=16/24") {
		t.Fatalf("contract drift error = %v", err)
	}
}

func TestReceiptLoadsLegacySchemaAndFailsItStale(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "receipt.json")
	content := `{
  "schema_version": 1,
  "decision": "accept-focused-structural-matches",
  "mori_version": "0.29.0",
  "normalization_version": 12,
  "head_commit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "index_digest": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "scan_profile_digest": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
  "focused_match_ids": ["aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbb"]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	receipt, err := Load(path)
	if err != nil {
		t.Fatalf("Load legacy: %v", err)
	}
	evidence := testEvidence()
	evidence.FocusedMatchIDs = []string{"aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbb"}
	if err := Validate(receipt, evidence); err == nil || !strings.Contains(err.Error(), "contract is legacy and stale") {
		t.Fatalf("legacy validation error = %v", err)
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
		StagedReviewContract: StagedReviewContract{
			IncludeFocused:       true,
			RequireFocusCoverage: true,
			RequiredFocusedFiles: 24,
			CoveredFocusedFiles:  24,
		},
	}
}
