package parser

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/source"
)

// Whitespace must not change the cast/coalescing profile. Casts use their
// original grammar tree rather than synthetic parentheses.
func TestSwiftMethodCastPreservesWhitespaceProfile(t *testing.T) {
	t.Parallel()
	for _, expression := range []string{
		`defaults.object(forKey: "opacity") as? Double`,
		`defaults.object(forKey: key) as? Bool`,
		`settings.defaults.object(forKey: "opacity") as? Double`,
	} {
		t.Run(expression, func(t *testing.T) {
			parse := func(expression string) model.Fragment {
				t.Helper()
				path := filepath.Join(t.TempDir(), "input.swift")
				content := "func load() {\n let value = " + expression + " ?? fallback\n consume(value)\n}\n"
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				spec, _ := language.Detect(path)
				fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.swift", Language: spec}, 1)
				if len(warnings) != 0 || len(fragments) != 1 {
					t.Fatalf("fragments/warnings: %+v / %+v", fragments, warnings)
				}
				return fragments[0]
			}
			repaired := parse(expression)
			explicit := parse(" " + expression + " ")
			if repaired.Fingerprint != explicit.Fingerprint || !reflect.DeepEqual(repaired.Features, explicit.Features) || repaired.TokenCount != explicit.TokenCount {
				t.Fatalf("whitespace changed normalized profile: got=%s want=%s", repaired.Fingerprint, explicit.Fingerprint)
			}
		})
	}
}

func TestSwiftCastInCallArgumentPreservesWhitespaceProfile(t *testing.T) {
	t.Parallel()
	for _, expression := range []string{
		`data[key] as? String`,
		`advertisementData[key] as? String`,
		`defaults.object(forKey: "opacity") as? Double`,
	} {
		t.Run(expression, func(t *testing.T) {
			parse := func(expression string) model.Fragment {
				t.Helper()
				path := filepath.Join(t.TempDir(), "input.swift")
				content := "func load() { if check(" + expression + " ?? fallback) { work() } }"
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				spec, _ := language.Detect(path)
				fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.swift", Language: spec}, 1)
				if len(warnings) != 0 || len(fragments) != 1 {
					t.Fatalf("fragments/warnings: %+v / %+v", fragments, warnings)
				}
				return fragments[0]
			}
			repaired := parse(expression)
			explicit := parse(" " + expression + " ")
			if repaired.Fingerprint != explicit.Fingerprint || !reflect.DeepEqual(repaired.Features, explicit.Features) {
				t.Fatalf("argument whitespace changed profile: got=%s want=%s", repaired.Fingerprint, explicit.Fingerprint)
			}
		})
	}
}

func TestSwiftOptionalTypesAndCoalescingKeepDistinctShapes(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"double_optional":  `func check(value: AnyObject??) { consume(value) }`,
		"triple_optional":  `func check(value: AnyObject???) { consume(value) }`,
		"cast_in_call":     `func check() { if matches(data[key] as? String ?? fallback) { consume() } }`,
		"method_reference": `func check() { consume(object.method(forKey:) as? Any ?? fallback) }`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.swift")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.swift", Language: spec}, 1)
			if len(warnings) != 0 || len(fragments) != 1 {
				t.Fatalf("fragments=%d warnings=%+v", len(fragments), warnings)
			}
			if name == "cast_in_call" || name == "method_reference" {
				features := fragments[0].Features
				if features["edge:expression:nil_coalescing>expression:as"] != 1 || features["node:expression:tuple"] != 0 {
					t.Fatalf("cast must be the coalescing operand without synthetic grouping: %+v", features)
				}
			}
		})
	}
}
