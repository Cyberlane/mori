package support

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func fixture() Session {
	return Session{SchemaVersion: 1, ReportAvailable: true, Tool: Build{Version: "0.33.0", Revision: strings.Repeat("a", 40), GOOS: "darwin", GOARCH: "arm64", GoVersion: "go1.26.6", ReportSchema: 22, NormalizationVersion: 14, ConfigSchema: 2, ContractSchema: 1}, Status: "success", Settings: &Settings{Profile: "review", Threshold: .85, MinTokens: 40, MaxZeroFragmentFiles: -1, Ranking: "review", ComparisonDomain: "code", SQLDialect: "generic", FragmentSelection: "all"}, Counts: Counts{Files: 2, Fragments: 3, MatchGroups: 1, LeadingGroups: 1}, Phases: []Phase{{Name: "total", Milliseconds: 42}}, Languages: []LanguageCount{{Language: "typescript", Files: 2, Fragments: 3}}}
}
func TestRoundTripAndPrivateDeterministicBundle(t *testing.T) {
	s := fixture()
	s.ReviewedFindings = []ReviewedFinding{{Rank: 7, Classification: "intentional"}, {Rank: 1, Classification: "useful"}}
	root := t.TempDir()
	plain := filepath.Join(root, "session.json")
	archive := filepath.Join(root, "support.zip")
	if err := WriteSession(plain, s); err != nil {
		t.Fatal(err)
	}
	if err := WriteBundle(archive, s); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plain, archive} {
		got, err := Inspect(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, canonical(s)) {
			t.Fatalf("round trip %+v", got)
		}
		info, _ := os.Stat(path)
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			t.Fatalf("not private: %v", info.Mode())
		}
	}
	first, _ := os.ReadFile(archive)
	second, err := bundle(s)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("archive not deterministic")
	}
	if err := WriteBundle(archive, s); err == nil {
		t.Fatal("overwrote existing archive")
	}
	unchanged, _ := os.ReadFile(archive)
	if !bytes.Equal(first, unchanged) {
		t.Fatal("existing output changed")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 2 {
		t.Fatalf("temporary files leaked %v", entries)
	}
}
func TestValidationRejectsPrivacySentinelsAndInvalidValues(t *testing.T) {
	sentinel := "PRIVATE_REPO_/Users/person/token=secret"
	cases := []struct {
		name string
		edit func(*Session)
	}{
		{"version", func(s *Session) { s.Tool.Version = sentinel }}, {"revision", func(s *Session) { s.Tool.Revision = sentinel }}, {"runtime", func(s *Session) { s.Tool.GoVersion = sentinel }}, {"os", func(s *Session) { s.Tool.GOOS = sentinel }}, {"arch", func(s *Session) { s.Tool.GOARCH = sentinel }},
		{"status", func(s *Session) { s.Status = sentinel }}, {"failure", func(s *Session) { s.Status = "error"; s.Failure = &Failure{Stage: "analysis", Code: sentinel} }}, {"stage", func(s *Session) { s.Status = "error"; s.Failure = &Failure{Stage: sentinel, Code: "unknown"} }},
		{"profile", func(s *Session) { s.Settings.Profile = sentinel }}, {"domain", func(s *Session) { s.Settings.ComparisonDomain = sentinel }}, {"dialect", func(s *Session) { s.Settings.SQLDialect = sentinel }}, {"selection", func(s *Session) { s.Settings.FragmentSelection = sentinel }}, {"ranking", func(s *Session) { s.Settings.Ranking = sentinel }},
		{"phase", func(s *Session) { s.Phases[0].Name = sentinel }}, {"language", func(s *Session) { s.Languages[0].Language = sentinel }}, {"classification", func(s *Session) { s.ReviewedFindings = []ReviewedFinding{{Rank: 1, Classification: sentinel}} }},
		{"nan", func(s *Session) { s.Settings.Threshold = math.NaN() }}, {"infinity", func(s *Session) { s.Settings.MinFileCoverage = math.Inf(1) }}, {"negative", func(s *Session) { s.Counts.Files = -1 }}, {"duration", func(s *Session) { s.Phases[0].Milliseconds = 604800001 }},
		{"duplicate-language", func(s *Session) { s.Languages = append(s.Languages, s.Languages[0]) }}, {"duplicate-phase", func(s *Session) { s.Phases = append(s.Phases, s.Phases[0]) }}, {"duplicate-rank", func(s *Session) {
			s.ReviewedFindings = []ReviewedFinding{{Rank: 1, Classification: "useful"}, {Rank: 1, Classification: "uncertain"}}
		}}, {"too-many-labels", func(s *Session) {
			for i := 1; i <= 26; i++ {
				s.ReviewedFindings = append(s.ReviewedFindings, ReviewedFinding{Rank: i, Classification: "useful"})
			}
		}},
		{"unavailable", func(s *Session) { s.ReportAvailable = false }}, {"composition", func(s *Session) { s.Counts.LeadingTestStoryGroups = 2 }}, {"leading-bound", func(s *Session) { s.Counts.LeadingGroups = 26 }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := fixture()
			tt.edit(&s)
			data, err := Marshal(s)
			if err == nil {
				t.Fatal("accepted invalid private field")
			}
			if bytes.Contains(data, []byte(sentinel)) || strings.Contains(err.Error(), sentinel) {
				t.Fatal("private value leaked in result/error")
			}
		})
	}
	b := fixture().Tool
	b.Version = sentinel
	b.Revision = sentinel
	b.GoVersion = sentinel
	b.GOOS = sentinel
	b.GOARCH = sentinel
	b = SanitizeBuild(b)
	if b.Version != "unknown" || b.Revision != "unknown" || b.GoVersion != "unknown" || b.GOOS != "unknown" || b.GOARCH != "unknown" {
		t.Fatal("build metadata not sanitized")
	}
}
func TestStrictJSONAndFailures(t *testing.T) {
	data, _ := Marshal(fixture())
	cases := [][]byte{bytes.Replace(data, []byte(`"files": 2`), []byte(`"files": 9223372036854775808`), 1), bytes.Replace(data, []byte(`"status": "success"`), []byte(`"status": "success", "Status": "error"`), 1), bytes.Replace(data, []byte(`"files": 2`), []byte(`"files": 2, "Files": 3`), 1), append(append([]byte{}, data...), []byte(` {}`)...), bytes.Replace(data, []byte(`"schema_version": 1`), []byte(`"schema_version": 1,"source":"PRIVATE_SOURCE"`), 1), bytes.Replace(data, []byte(`"schema_version": 1`), []byte(`"schema_version": 1,"schema_version":1`), 1), bytes.Replace(data, []byte(`"schema_version": 1,`), nil, 1), bytes.Replace(data, []byte(`"report_available": true`), []byte(`"report_available": null`), 1), []byte(strings.Repeat(" ", MaxSessionBytes+1)), bytes.Replace(data, []byte(`"files": 2`), []byte(`"files": 2.2`), 1)}
	for i, b := range cases {
		if _, err := decode(b); err == nil {
			t.Fatalf("accepted malformed JSON %d", i)
		}
	}
	s := fixture()
	s.ReportAvailable = false
	s.Counts = Counts{}
	s.Languages = nil
	s.Settings = nil
	s.Status = "usage"
	s.Failure = &Failure{Code: "invalid-arguments", Stage: "arguments"}
	if _, err := Marshal(s); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"", "default", "review", "explore", "sql"} {
		s := fixture()
		s.Settings.Profile = profile
		s.Settings.Ranking = "structural"
		if _, err := Marshal(s); err != nil {
			t.Fatalf("valid profile %q rejected", profile)
		}
	}
}
func TestRejectsUnsafeFilesAndRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	if err := WriteSession(target, fixture()); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, err := ReadSession(link); err == nil {
		t.Fatal("read final symlink")
	}
	if err := WriteSession(link, fixture()); err == nil {
		t.Fatal("wrote symlink")
	}
	if _, err := ReadSession(root); err == nil {
		t.Fatal("read directory")
	}
	large := filepath.Join(root, "large")
	if err := os.WriteFile(large, make([]byte, MaxBundleBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(large); err == nil {
		t.Fatal("read oversized artifact")
	}
	missing := filepath.Join(root, "missing", "session.json")
	if err := WriteSession(missing, fixture()); err == nil {
		t.Fatal("created implicit parent")
	}
}
func TestAdversarialArchives(t *testing.T) {
	data, _ := Marshal(fixture())
	makeZIP := func(names []string, mode os.FileMode, content []byte, method uint16) []byte {
		var b bytes.Buffer
		w := zip.NewWriter(&b)
		for _, name := range names {
			h := &zip.FileHeader{Name: name, Method: method}
			h.SetMode(mode)
			f, _ := w.CreateHeader(h)
			f.Write(content)
		}
		w.Close()
		return b.Bytes()
	}
	valid, _ := bundle(fixture())
	cases := [][]byte{
		makeZIP([]string{"../session.json", "README.txt"}, 0600, data, zip.Store), makeZIP([]string{"session.json", "session.json"}, 0600, data, zip.Store), makeZIP([]string{"session.json", "README.txt", "secret.txt"}, 0600, data, zip.Store), makeZIP([]string{"session.json", "README.txt"}, os.ModeSymlink|0600, data, zip.Store), makeZIP([]string{"session.json", "README.txt"}, 0600, make([]byte, MaxSessionBytes+1), zip.Deflate), makeZIP([]string{"session.json", "README.txt"}, 0600, data, zip.Store), append(append([]byte{}, valid...), []byte("PRIVATE_TRAILING_DATA")...), append([]byte("PRIVATE_PREFIX"), valid...), valid[:len(valid)-4],
	}
	for i, b := range cases {
		if _, err := decodeBundle(b); err == nil {
			t.Fatalf("accepted unsafe archive %d", i)
		}
	}
}
func TestSchemaMatchesTypedAllowlist(t *testing.T) {
	data, err := os.ReadFile("../../schemas/mori-support-session-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err = json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	defs := schema["$defs"].(map[string]any)
	for _, value := range []any{Session{}, Build{}, Settings{}, Counts{}, Failure{}, Phase{}, LanguageCount{}, ReviewedFinding{}} {
		typ := reflect.TypeOf(value)
		def := defs[typ.Name()].(map[string]any)
		if def["additionalProperties"] != false {
			t.Fatalf("unbounded object %s", typ.Name())
		}
		props := def["properties"].(map[string]any)
		if len(props) != typ.NumField() {
			t.Fatalf("schema fields differ for %s", typ.Name())
		}
		required := map[string]bool{}
		for _, name := range def["required"].([]any) {
			required[name.(string)] = true
		}
		for i := 0; i < typ.NumField(); i++ {
			parts := strings.Split(typ.Field(i).Tag.Get("json"), ",")
			if _, ok := props[parts[0]]; !ok {
				t.Fatalf("missing %s.%s", typ.Name(), parts[0])
			}
			optional := len(parts) > 1
			if required[parts[0]] == optional {
				t.Fatalf("required mismatch %s.%s", typ.Name(), parts[0])
			}
		}
	}
}

func TestSchemaMetadataValidationMatchesRuntime(t *testing.T) {
	data, err := os.ReadFile("../../schemas/mori-support-session-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err = decoder.Decode(&schema); err != nil {
		t.Fatal(err)
	}
	defs := schema["$defs"].(map[string]any)
	prop := func(typ, name string) map[string]any {
		return defs[typ].(map[string]any)["properties"].(map[string]any)[name].(map[string]any)
	}
	for name, pattern := range map[string]string{"version": versionPattern.String(), "revision": revisionPattern.String(), "go_version": goPattern.String()} {
		if prop("Build", name)["pattern"] != pattern {
			t.Fatalf("schema pattern differs for %s", name)
		}
	}
	for key, want := range map[string][]string{"Build.goos": osValues, "Build.goarch": archValues, "LanguageCount.language": languageValues} {
		parts := strings.Split(key, ".")
		raw := prop(parts[0], parts[1])["enum"].([]any)
		var got []string
		for _, v := range raw {
			got = append(got, v.(string))
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("enum differs for %s", key)
		}
	}
	for _, typ := range []string{"Counts", "Settings", "Phase", "Build", "ReviewedFinding"} {
		for name, raw := range defs[typ].(map[string]any)["properties"].(map[string]any) {
			p := raw.(map[string]any)
			if p["type"] != "integer" {
				continue
			}
			lo, hi := int64(0), maxInteger
			switch {
			case typ == "Build" || typ == "ReviewedFinding":
				lo = 1
				hi = 1000000
			case typ == "Phase":
				hi = 604800000
			case typ == "Counts" && strings.HasPrefix(name, "leading_"):
				hi = 25
			case typ == "Settings" && name == "max_zero_fragment_files":
				lo = -1
			}
			gotLo, lowErr := p["minimum"].(json.Number).Int64()
			gotHi, highErr := p["maximum"].(json.Number).Int64()
			if lowErr != nil || highErr != nil || gotLo != lo || gotHi != hi {
				t.Fatalf("numeric bound mismatch %s.%s", typ, name)
			}
		}
	}
}

func TestExactInt64CountsAndSettingsRoundTrip(t *testing.T) {
	s := fixture()
	s.Settings.Workers = math.MaxInt64
	s.Counts.CandidatePairs = math.MaxInt64
	s.Languages[0].Fragments = math.MaxInt64
	data, err := Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"workers": 9223372036854775807`)) {
		t.Fatal("integer digits lost")
	}
	got, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.Workers != math.MaxInt64 || got.Counts.CandidatePairs != math.MaxInt64 || got.Languages[0].Fragments != math.MaxInt64 {
		t.Fatal("int64 round trip lost precision")
	}
}
