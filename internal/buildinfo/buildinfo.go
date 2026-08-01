// Package buildinfo holds release metadata injected by linker flags or Go
// module build information.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// These values are replaced in release builds.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	Version, Commit, Date = resolve(Version, Commit, Date, info)
}

func resolve(version string, commit string, date string, info *debug.BuildInfo) (string, string, string) {
	if info == nil {
		return version, commit, date
	}
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = strings.TrimPrefix(info.Main.Version, "v")
	}

	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if commit == "unknown" {
		if revision := settings["vcs.revision"]; revision != "" {
			commit = revision
			if len(commit) > 12 {
				commit = commit[:12]
			}
			if settings["vcs.modified"] == "true" {
				commit += "-dirty"
			}
		}
	}
	if date == "unknown" && settings["vcs.time"] != "" {
		date = settings["vcs.time"]
	}
	return version, commit, date
}
