// Command releasepack creates one native release archive.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Cyberlane/mori/internal/release"
)

func main() {
	binary := flag.String("binary", "", "path to the built mori binary")
	root := flag.String("root", ".", "repository root")
	output := flag.String("output", "dist", "archive output directory")
	version := flag.String("version", "", "release version")
	goos := flag.String("goos", "", "target GOOS")
	goarch := flag.String("goarch", "", "target GOARCH")
	timestampValue := flag.String("timestamp", "", "RFC3339 release timestamp")
	flag.Parse()

	timestamp, err := time.Parse(time.RFC3339, *timestampValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "releasepack: parse timestamp: %v\n", err)
		os.Exit(1)
	}
	path, err := release.Package(release.Options{
		BinaryPath: *binary,
		Root:       *root,
		OutputDir:  *output,
		Version:    *version,
		GOOS:       *goos,
		GOARCH:     *goarch,
		Timestamp:  timestamp,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "releasepack: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(path)
}
