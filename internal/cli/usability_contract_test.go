package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/agentskill"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/normalize"
	"github.com/Cyberlane/mori/internal/projectcontract"
)

func TestPriorUsabilityContractUpgradePreservesProjectPolicy(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) { testPriorUsabilityContractUpgradePreservesProjectPolicy(t, version) })
	}
}

func testPriorUsabilityContractUpgradePreservesProjectPolicy(t *testing.T, version int) {
	t.Parallel()
	root := t.TempDir()
	policy := []byte(`{"profile":"review","exclude":["fixtures/**"],"scopes":{"library":{"roots":["src"]}}}`)
	if err := os.WriteFile(filepath.Join(root, ".mori.json"), policy, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &out, &stderr); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	contractPath := filepath.Join(root, projectcontract.FileName)
	old, _, err := projectcontract.Load(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	old.ConfigSchemaVersion = version
	old.ReportSchemaVersion = 21
	old.NormalizationVersion = 13
	raw, err := projectcontract.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &out, &stderr); code != 0 {
		t.Fatalf("%d %s %s", code, out.String(), stderr.String())
	}
	current, _, err := projectcontract.Load(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	skillDigest, err := agentskill.PackageDigest()
	if err != nil {
		t.Fatal(err)
	}
	if current.SchemaVersion != 1 || current.ConfigSchemaVersion != 3 || current.ReportSchemaVersion != model.SchemaVersion || current.NormalizationVersion != normalize.Version || current.EmbeddedSkill.Digest != skillDigest {
		t.Fatalf("contract %+v", current)
	}
	after, err := os.ReadFile(filepath.Join(root, ".mori.json"))
	if err != nil || !bytes.Equal(after, policy) {
		t.Fatal("upgrade changed user policy")
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"project", "upgrade", "--check", "--format", "json", root}, &out, &stderr); code != 0 {
		t.Fatalf("check %d %s", code, stderr.String())
	}
	var plan projectUpgradePlan
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Drift {
		t.Fatalf("drift %+v", plan)
	}
}
