package buildinfo

import (
	"runtime/debug"
	"testing"
)

const fullRevision = "a105dc574fc1402bb0463d16eb7d2d53d94fcd40"

func TestResolveUsesModuleAndVCSFallbacks(t *testing.T) {
	t.Parallel()

	info := resolve("dev", "unknown", "unknown", &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: fullRevision},
			{Key: "vcs.time", Value: "2026-07-29T06:48:29Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	})

	if info.Version != "0.1.0" || info.Revision != fullRevision || !info.Modified {
		t.Fatalf("build info = %#v, want module version and full dirty revision", info)
	}
	if info.SourceDate != "2026-07-29T06:48:29Z" {
		t.Fatalf("source date = %q, want VCS timestamp", info.SourceDate)
	}
	if got := DisplayRevision(info); got != "a105dc574fc1-dirty" {
		t.Fatalf("display revision = %q, want compact dirty revision", got)
	}
}

func TestResolvePreservesInjectedReleaseMetadata(t *testing.T) {
	t.Parallel()

	info := resolve("0.5.0", fullRevision, "release-date", &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "fallbackcommit"},
			{Key: "vcs.time", Value: "fallback-date"},
		},
	})

	if info.Version != "0.5.0" || info.Revision != fullRevision || info.SourceDate != "release-date" {
		t.Fatalf("metadata = %#v, want injected values", info)
	}
}

func TestResolveLeavesDevelopmentDefaultsWithoutBuildInfo(t *testing.T) {
	t.Parallel()

	info := resolve("dev", "unknown", "unknown", nil)
	if info.Version != "dev" || info.Revision != "unknown" || info.SourceDate != "unknown" {
		t.Fatalf("metadata = %#v, want development defaults", info)
	}
}
