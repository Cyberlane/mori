package projectcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarshalAndLoadAreDeterministic(t *testing.T) {
	contract := Contract{
		SchemaVersion: SchemaVersion, MoriVersion: "v0.30.0",
		EmbeddedSkill: Artifact{Revision: "v0.30.0", Digest: strings.Repeat("a", 64)},
		HookContract:  Artifact{Revision: "mori-hook-pre-commit/v1", Digest: strings.Repeat("b", 64)}, ConfigSchemaVersion: 1,
		ReportSchemaVersion: 20, ReviewReceiptSchema: 2, BaselineSchema: 4, NormalizationVersion: 12,
	}
	first, err := Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || !strings.HasSuffix(string(first), "\n") {
		t.Fatal("contract encoding is not deterministic")
	}
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	if _, err := WriteAtomic(path, first, false); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := Load(path)
	if err != nil || !exists || loaded != contract {
		t.Fatalf("load = %#v, %t, %v", loaded, exists, err)
	}
}

func TestLoadRejectsUnknownFieldsAndInvalidEvidence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	unknown := fmt.Sprintf(
		`{"schema_version":1,"mori_version":"v0.30.0","embedded_skill":{"revision":"v0.30.0","digest":"%s"},"hook_contract":{"revision":"v1","digest":"%s"},"config_schema_version":1,"report_schema_version":20,"review_receipt_schema_version":2,"baseline_schema_version":4,"normalization_version":12,"unknown":true}`,
		strings.Repeat("a", 64), strings.Repeat("b", 64),
	)
	if err := os.WriteFile(path, []byte(unknown), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}
	invalid := Contract{
		SchemaVersion: SchemaVersion, MoriVersion: "v0.30.0",
		EmbeddedSkill:       Artifact{Revision: "v0.30.0", Digest: strings.Repeat("A", 64)},
		HookContract:        Artifact{Revision: "v1", Digest: strings.Repeat("b", 64)},
		ConfigSchemaVersion: 1, ReportSchemaVersion: 20, ReviewReceiptSchema: 2,
		BaselineSchema: 4, NormalizationVersion: 12,
	}
	if _, err := Marshal(invalid); err == nil {
		t.Fatal("Marshal accepted non-canonical digest")
	}
}

func TestLoadRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	if err := os.Symlink(filepath.Join(root, "missing"), path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Load symlink error = %v", err)
	}
}
