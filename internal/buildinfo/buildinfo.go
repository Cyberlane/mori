// Package buildinfo holds immutable build provenance from linker flags or Go
// module build information.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// These values are replaced in release builds.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// Info is deterministic provenance for the running Mori binary.
type Info struct {
	Name                 string `json:"name"`
	Version              string `json:"version"`
	Revision             string `json:"revision"`
	SourceDate           string `json:"source_date"`
	Modified             bool   `json:"modified"`
	GoVersion            string `json:"go_version"`
	GOOS                 string `json:"goos"`
	GOARCH               string `json:"goarch"`
	NormalizationVersion int    `json:"normalization_version"`
}

var current = resolve(Version, Commit, Date, readBuildInfo())

func readBuildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info
}

// Current returns the immutable provenance of the running binary.
func Current() Info {
	return current
}

func resolve(version string, commit string, date string, info *debug.BuildInfo) Info {
	result := Info{
		Name:       "mori",
		Version:    version,
		Revision:   commit,
		SourceDate: date,
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}
	if info == nil {
		return result
	}
	if result.Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		result.Version = strings.TrimPrefix(info.Main.Version, "v")
	}

	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if result.Revision == "unknown" && settings["vcs.revision"] != "" {
		result.Revision = settings["vcs.revision"]
	}
	result.Modified = settings["vcs.modified"] == "true"
	if result.SourceDate == "unknown" && settings["vcs.time"] != "" {
		result.SourceDate = settings["vcs.time"]
	}
	return result
}

// DisplayRevision returns a compact revision for human-facing version output.
func DisplayRevision(info Info) string {
	revision := info.Revision
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if info.Modified {
		revision += "-dirty"
	}
	return revision
}
