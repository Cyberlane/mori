package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/baseline"
)

func TestPriorNormalizationMigrationRequiresExplicitCompleteScan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"left", "right"} {
		if err := os.WriteFile(filepath.Join(root, name+".go"), []byte("package example\nfunc "+name+"(x int) int { return x + 1 }\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "baseline.json")
	common := []string{"--no-config", "--baseline", path, "--min-tokens", "1", root}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), append([]string{"baseline", "update", "--accept-all"}, common...), &out, &stderr); code != exitSuccess {
		t.Fatalf("create baseline %d %s", code, stderr.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["normalization_version"] = float64(12)
	entries := document["entries"].([]any)
	if len(entries) == 0 {
		t.Fatal("fixture has no decisions")
	}
	entry := entries[0].(map[string]any)
	entry["classification"] = "intentional"
	entry["note"] = "preserve reviewed decision"
	old, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), append([]string{"scan"}, common...), &out, &stderr); code != exitError || !strings.Contains(stderr.String(), "normalization version 12") {
		t.Fatalf("old scan %d %s", code, stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), append([]string{"baseline", "migrate"}, common...), &out, &stderr); code != exitUsage {
		t.Fatalf("implicit migration %d", code)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.go"), []byte("package example\nfunc Broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	migrate := append([]string{"baseline", "migrate", "--accept-profile"}, common...)
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), migrate, &out, &stderr); code != exitCoverage {
		t.Fatalf("incomplete migration %d %s", code, stderr.String())
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, old) {
		t.Fatal("rejected migration rewrote baseline")
	}
	if err := os.Remove(filepath.Join(root, "broken.go")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), migrate, &out, &stderr); code != exitSuccess || !strings.Contains(out.String(), "currently stale") {
		t.Fatalf("migration %d %s %s", code, out.String(), stderr.String())
	}
	set, err := baseline.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := set.Entries(); len(got) != len(entries) || got[0].Classification != "intentional" || got[0].Note != "preserve reviewed decision" {
		t.Fatalf("lost decisions: %+v", got)
	}
}
