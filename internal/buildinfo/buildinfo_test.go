package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolveUsesModuleAndVCSFallbacks(t *testing.T) {
	t.Parallel()

	version, commit, date := resolve("dev", "unknown", "unknown", &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "a105dc574fc1402bb0463d16eb7d2d53d94fcd40"},
			{Key: "vcs.time", Value: "2026-07-29T06:48:29Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	})

	if version != "0.1.0" {
		t.Fatalf("version = %q, want 0.1.0", version)
	}
	if commit != "a105dc574fc1-dirty" {
		t.Fatalf("commit = %q, want shortened dirty revision", commit)
	}
	if date != "2026-07-29T06:48:29Z" {
		t.Fatalf("date = %q, want VCS timestamp", date)
	}
}

func TestResolvePreservesInjectedReleaseMetadata(t *testing.T) {
	t.Parallel()

	version, commit, date := resolve("0.2.0", "releasecommit", "release-date", &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "fallbackcommit"},
			{Key: "vcs.time", Value: "fallback-date"},
		},
	})

	if version != "0.2.0" || commit != "releasecommit" || date != "release-date" {
		t.Fatalf("metadata = %q/%q/%q, want injected values", version, commit, date)
	}
}

func TestResolveLeavesDevelopmentDefaultsWithoutBuildInfo(t *testing.T) {
	t.Parallel()

	version, commit, date := resolve("dev", "unknown", "unknown", nil)
	if version != "dev" || commit != "unknown" || date != "unknown" {
		t.Fatalf("metadata = %q/%q/%q, want development defaults", version, commit, date)
	}
}
