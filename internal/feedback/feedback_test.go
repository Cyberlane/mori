package feedback

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Cyberlane/mori/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(base, "settings"), base)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func sample() Sample {
	return Measure(model.Report{Files: 15, Fragments: 4, TotalMatchGroups: 2}, time.Second, "findings")
}
func TestConsentAndRetention(t *testing.T) {
	s := testStore(t)
	if err := s.Capture(sample(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.directory); !os.IsNotExist(err) {
		t.Fatal("disabled capture created storage")
	}
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	if err := s.Capture(sample(), true); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Read()
	if len(state.Samples) != 0 {
		t.Fatal("CI consent inferred")
	}
	for i := 0; i < MaxRecords+3; i++ {
		if err := s.Capture(sample(), false); err != nil {
			t.Fatal(err)
		}
	}
	state, _ = s.Read()
	if len(state.Samples) != MaxRecords {
		t.Fatal("retention limit not enforced")
	}
	if err := s.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := s.Capture(sample(), false); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Read()
	if state.Enabled || state.CI || len(state.Samples) != MaxRecords {
		t.Fatal("disable lost history or consent")
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Read()
	if len(state.Samples) != 0 || state.Enabled {
		t.Fatal("clear changed consent")
	}
	if err := s.Enable(true); err != nil {
		t.Fatal(err)
	}
	if err := s.Capture(sample(), true); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Read()
	if len(state.Samples) != 1 {
		t.Fatal("explicit CI consent ignored")
	}
}
func TestExportAllowlistAndPrivacy(t *testing.T) {
	s := testStore(t)
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	r := model.Report{Files: 17, Fragments: 240, Configuration: model.EffectiveConfig{ConfigPath: "SECRET-PATH", ScanProfileDigest: "SECRET-HASH"}}
	if err := s.Capture(Measure(r, time.Second, "SECRET-NOTE"), false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.Export(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "SECRET") || strings.Contains(out.String(), s.directory) {
		t.Fatal("private data exported")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["schema_version"] == nil || payload["samples"] == nil {
		t.Fatal("unexpected export field")
	}
	var samples []map[string]json.RawMessage
	_ = json.Unmarshal(payload["samples"], &samples)
	if len(samples) != 1 || len(samples[0]) != 13 {
		t.Fatal("unexpected sample fields")
	}
}
func TestRejectUnsafeState(t *testing.T) {
	for _, kind := range []string{"symlink", "permissions", "unknown", "enumeration", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && (kind == "permissions" || kind == "symlink") {
				t.Skip("requires POSIX mode bits or symlink privilege")
			}
			s := testStore(t)
			if err := s.Enable(false); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(s.directory, "state.json")
			switch kind {
			case "symlink":
				_ = os.Remove(name)
				if err := os.Symlink(filepath.Join(s.directory, "missing"), name); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				_ = os.Chmod(name, 0644)
			case "unknown":
				_ = os.WriteFile(name, []byte(`{"schema_version":1,"secret":"do not export"}`), 0600)
			case "enumeration":
				state := State{SchemaVersion: SchemaVersion, Samples: []Sample{sample()}}
				state.Samples[0].Files = "PRIVATE"
				data, _ := json.Marshal(state)
				_ = os.WriteFile(name, data, 0600)
			case "oversize":
				_ = os.WriteFile(name, bytes.Repeat([]byte(" "), maxBytes+1), 0600)
			}
			var out bytes.Buffer
			if err := s.Export(&out); err == nil || out.Len() != 0 {
				t.Fatal("unsafe state exported")
			}
		})
	}
}
func TestProjectIsolationAndStorageSymlink(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "project")
	_ = os.Mkdir(project, 0700)
	a, _ := Open(filepath.Join(base, "settings"), base)
	b, _ := Open(filepath.Join(base, "settings"), project)
	if a.directory == b.directory {
		t.Fatal("project consent shared")
	}
	if err := a.Enable(false); err != nil {
		t.Fatal(err)
	}
	state, err := b.Read()
	if err != nil || state.Enabled {
		t.Fatal("project consent inherited")
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	link := filepath.Join(base, "link")
	_ = os.Symlink(filepath.Join(base, "settings"), link)
	unsafe, _ := Open(link, base)
	if err := unsafe.Enable(false); err == nil {
		t.Fatal("storage symlink followed")
	}
}

func TestExplicitClassificationAndCache(t *testing.T) {
	s := testStore(t)
	if err := s.ClassifyLatest("useful", "", 1, false); err == nil {
		t.Fatal("classification enrolled project")
	}
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	if err := s.ClassifyLatest("useful", "", 1, false); err == nil {
		t.Fatal("classified absent scan")
	}
	r := model.Report{TotalMatchGroups: 1}
	r.Tool.Version = "v1.2.3"
	measured := Measure(r, 6*time.Minute, "completed", "hit")
	if measured.ToolVersion != "1.2.3" || measured.Cache != "hit" || measured.Duration != "5m+" || measured.Classification != "" {
		t.Fatal("invalid measurement")
	}
	if err := s.Capture(measured, false); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []struct {
		c, e string
		r    int
	}{{"SECRET", "", 1}, {"useful", "SECRET", 1}, {"useful", "", -1}} {
		if err := s.ClassifyLatest(bad.c, bad.e, bad.r, false); err == nil {
			t.Fatal("untrusted classification accepted")
		}
	}
	if err := s.ClassifyLatest("intentional", "1-5m", 7, true); err == nil {
		t.Fatal("CI consent bypassed")
	}
	if err := s.ClassifyLatest("intentional", "1-5m", 7, false); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Read()
	latest := state.Samples[0]
	if latest.Classification != "intentional" || latest.Effort != "1-5m" || latest.ReviewedRank != "6-10" {
		t.Fatal("explicit classification not stored")
	}
	r.Tool.Version = "1.2.3-PRIVATE"
	if Measure(r, 0, "completed").ToolVersion != "dev" {
		t.Fatal("private version exposed")
	}
	if err := s.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := s.ClassifyLatest("useful", "", 0, false); err == nil {
		t.Fatal("disabled classification accepted")
	}
}

func TestLanguageAllowlistAndBusyStore(t *testing.T) {
	s := testStore(t)
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	r := model.Report{FileCoverage: []model.FileCoverage{{Language: "PRIVATE-LANGUAGE", Path: "PRIVATE-PATH"}, {Language: "go"}}}
	measured := Measure(r, 2*time.Minute, "completed", "PRIVATE-CACHE")
	if measured.Languages["other"] != "1-9" || measured.Languages["go"] != "1-9" || measured.Duration != "1-5m" || measured.Cache != "bypassed" {
		t.Fatal("invalid coarse language measurement")
	}
	if err := s.Capture(measured, false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.Export(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "PRIVATE") {
		t.Fatal("private metadata exported")
	}
	if err := os.Mkdir(filepath.Join(s.directory, "lock"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Capture(measured, false); err == nil {
		t.Fatal("concurrent writer lock ignored")
	}
	measured.Languages["PRIVATE"] = "1-9"
	if validSample(measured) {
		t.Fatal("unknown language enum accepted")
	}
}

func TestPolicyAndScopeMeasurements(t *testing.T) {
	r := model.Report{TotalMatchGroups: 123, TotalFocusedMatchGroups: 3, Review: &model.ReviewOutcome{Policy: "advisory", Analysis: "incomplete"}, Configuration: model.EffectiveConfig{Focus: &model.FocusConfig{}}, Warnings: []model.Warning{{Kind: "parse", Message: "PRIVATE"}, {Kind: "PRIVATE", Path: "PRIVATE"}}}
	measured := Measure(r, 0, "completed")
	if measured.Policy != "advisory" || measured.Analysis != "incomplete" || measured.FindingsScope != "focused" || measured.Findings != "1-9" || measured.WarningCategories["parse"] != "1-9" || measured.WarningCategories["other"] != "1-9" {
		t.Fatal("incorrect policy, analysis or focused measurement")
	}
	s := testStore(t)
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	if err := s.Capture(measured, false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := s.Export(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "PRIVATE") {
		t.Fatal("untrusted warning exposed")
	}
	r.Review = nil
	r.Configuration.Focus = nil
	measured = Measure(r, 0, "completed")
	if measured.Policy != "scan" || measured.Analysis != "unknown" || measured.FindingsScope != "all" || measured.Findings != "100-999" {
		t.Fatal("ordinary scan inferred analysis completion")
	}
	r.Review = &model.ReviewOutcome{Policy: "PRIVATE", Analysis: "PRIVATE"}
	measured = Measure(r, 0, "completed")
	if measured.Policy != "scan" || measured.Analysis != "unknown" {
		t.Fatal("untrusted review metadata passed")
	}
	measured.WarningCategories["PRIVATE"] = "1-9"
	if validSample(measured) {
		t.Fatal("unknown warning category accepted")
	}
}

func TestStaleLockRecoveryAndBoundedTempCleanup(t *testing.T) {
	s := testStore(t)
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	if err := s.Capture(sample(), false); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(s.directory, "lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Disable(); err == nil {
		t.Fatal("recent writer lock removed")
	}
	old := time.Now().Add(-11 * time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	oldTemp := filepath.Join(s.directory, ".state-orphan")
	newTemp := filepath.Join(s.directory, ".state-recent")
	unrelated := filepath.Join(s.directory, "unrelated")
	for _, path := range []string{oldTemp, newTemp, unrelated} {
		if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{oldTemp, unrelated} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Disable(); err != nil {
		t.Fatal(err)
	}
	state, err := s.Read()
	if err != nil || state.Enabled {
		t.Fatal("stale lock prevented disable")
	}
	if _, err := os.Stat(oldTemp); !os.IsNotExist(err) {
		t.Fatal("stale owned temp retained")
	}
	for _, path := range []string{newTemp, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("unrelated or recent file removed")
		}
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
}
func TestStaleLockDoesNotRemoveNonemptyOrSymlink(t *testing.T) {
	s := testStore(t)
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(s.directory, "lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(lock, "unexpected")
	if err := os.WriteFile(child, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-11 * time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if err := s.Disable(); err == nil {
		t.Fatal("nonempty stale lock removed")
	}
	if _, err := os.Stat(child); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges")
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(s.directory, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, lock); err != nil {
		t.Fatal(err)
	}
	if err := s.Disable(); err == nil {
		t.Fatal("symlink lock followed")
	}
	info, err := os.Lstat(lock)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink removed")
	}
}

func TestClassificationRejectsZeroFindings(t *testing.T) {
	s := testStore(t)
	if err := s.Enable(false); err != nil {
		t.Fatal(err)
	}
	if err := s.Capture(Measure(model.Report{}, 0, "completed"), false); err != nil {
		t.Fatal(err)
	}
	if err := s.ClassifyLatest("useful", "under-1m", 1, false); err == nil {
		t.Fatal("classified nonexistent finding")
	}
	state, err := s.Read()
	if err != nil || state.Samples[0].Classification != "" {
		t.Fatal("failed classification mutated sample", err)
	}
}
