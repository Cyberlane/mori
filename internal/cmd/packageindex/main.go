// Command packageindex creates package-manager manifests for release archives.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Cyberlane/mori/internal/release"
)

func main() {
	dist := flag.String("dist", "dist", "directory containing native release archives")
	version := flag.String("version", "", "release version without a v prefix")
	flag.Parse()

	paths, err := release.GeneratePackageIndexes(release.IndexOptions{DistDir: *dist, Version: *version})
	if err != nil {
		fmt.Fprintf(os.Stderr, "packageindex: %v\n", err)
		os.Exit(1)
	}
	for _, path := range paths {
		fmt.Println(path)
	}
}
