// Package buildinfo holds release metadata injected by linker flags.
package buildinfo

// These values are replaced in release builds.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
