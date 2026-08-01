// Command skillpack creates Mori's portable Agent Skill release archive.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Cyberlane/mori/internal/release"
)

func main() {
	root := flag.String("root", ".", "repository root")
	output := flag.String("output", "dist", "archive output directory")
	version := flag.String("version", "", "release version")
	timestampValue := flag.String("timestamp", "", "RFC3339 release timestamp")
	flag.Parse()

	timestamp, err := time.Parse(time.RFC3339, *timestampValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skillpack: parse timestamp: %v\n", err)
		os.Exit(1)
	}
	path, err := release.PackageSkill(release.SkillOptions{
		Root:      *root,
		OutputDir: *output,
		Version:   *version,
		Timestamp: timestamp,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "skillpack: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(path)
}
