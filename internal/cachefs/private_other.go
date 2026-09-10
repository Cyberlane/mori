//go:build !windows

// Package cachefs creates and verifies private cache files without modifying
// permissions on existing user-owned objects.
package cachefs

import "os"

func Private(path string, info os.FileInfo) bool { return info.Mode().Perm()&0077 == 0 }
func Create(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}
func Mkdir(path string) error { return os.Mkdir(path, 0700) }
