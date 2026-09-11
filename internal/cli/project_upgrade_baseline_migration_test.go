package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyberlane/mori/internal/config"
)

func TestUpgradePreservesSupportedPriorNormalizationBaseline(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"left", "right"} {
		if err := os.WriteFile(filepath.Join(root, name+".go"), []byte("package example\nfunc "+name+"(x int) int { return x+1 }\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, ".mori-baseline.json")
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"baseline", "update", "--no-config", "--accept-all", "--baseline", path, "--min-tokens", "1", root}, &out, &stderr); code != exitSuccess {
		t.Fatalf("fixture %d %s", code, stderr.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["normalization_version"] = float64(12)
	prior, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, prior, 0600); err != nil {
		t.Fatal(err)
	}
	component := inspectProjectBaseline(root, config.Settings{}, false)
	if component.Status != "recommended" || component.Classification != "recommended" || !strings.Contains(component.Action, "mori baseline migrate --accept-profile") {
		t.Fatalf("supported migration: %+v", component)
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"project", "upgrade", "--apply", root}, &out, &stderr); code != exitSuccess {
		t.Fatalf("upgrade %d %s %s", code, out.String(), stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(prior, after) {
		t.Fatal("project upgrade changed protected baseline")
	}
	if component := inspectProjectBaseline(root, config.Settings{}, false); component.Status != "recommended" {
		t.Fatalf("post-upgrade status: %+v", component)
	}
	for _, invalid := range []string{"future", "corrupt-profile"} {
		t.Run(invalid, func(t *testing.T) {
			var bad map[string]any
			if err := json.Unmarshal(prior, &bad); err != nil {
				t.Fatal(err)
			}
			if invalid == "future" {
				bad["normalization_version"] = float64(15)
			} else {
				bad["scan_profile_digest"] = "corrupt"
			}
			raw, err := json.Marshal(bad)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if component := inspectProjectBaseline(root, config.Settings{}, false); component.Status != "conflict/manual" {
				t.Fatalf("invalid migration: %+v", component)
			}
		})
	}
}
