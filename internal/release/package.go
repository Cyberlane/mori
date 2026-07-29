// Package release creates deterministic native release archives.
package release

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Options identifies a built binary and its target archive.
type Options struct {
	BinaryPath string
	Root       string
	OutputDir  string
	Version    string
	GOOS       string
	GOARCH     string
	Timestamp  time.Time
}

type archiveFile struct {
	sourcePath string
	name       string
	mode       int64
	info       os.FileInfo
}

var safeIdentifier = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]*$`)

// Package writes a .zip for Windows or a .tar.gz for other platforms.
func Package(options Options) (string, error) {
	if err := validate(options); err != nil {
		return "", err
	}

	version := strings.TrimPrefix(options.Version, "v")
	baseName := fmt.Sprintf("mori_%s_%s_%s", version, options.GOOS, options.GOARCH)
	extension := ".tar.gz"
	if options.GOOS == "windows" {
		extension = ".zip"
	}
	archivePath := filepath.Join(options.OutputDir, baseName+extension)
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	binaryName := "mori"
	if options.GOOS == "windows" {
		binaryName += ".exe"
	}
	files := []archiveFile{
		{
			sourcePath: options.BinaryPath,
			name:       filepath.ToSlash(filepath.Join(baseName, binaryName)),
			mode:       0o755,
		},
		{
			sourcePath: filepath.Join(options.Root, "README.md"),
			name:       filepath.ToSlash(filepath.Join(baseName, "README.md")),
			mode:       0o644,
		},
		{
			sourcePath: filepath.Join(options.Root, "LICENSE"),
			name:       filepath.ToSlash(filepath.Join(baseName, "LICENSE")),
			mode:       0o644,
		},
		{
			sourcePath: filepath.Join(options.Root, "THIRD_PARTY_NOTICES.md"),
			name:       filepath.ToSlash(filepath.Join(baseName, "THIRD_PARTY_NOTICES.md")),
			mode:       0o644,
		},
	}
	for index := range files {
		info, err := os.Lstat(files[index].sourcePath)
		if err != nil {
			return "", fmt.Errorf("package %s: %w", files[index].sourcePath, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("package %s: input is not a regular file", files[index].sourcePath)
		}
		files[index].info = info
	}
	for _, file := range files[1:] {
		relative, err := filepath.Rel(options.Root, file.sourcePath)
		if err != nil {
			return "", fmt.Errorf("resolve package input %s: %w", file.sourcePath, err)
		}
		if relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("package input is outside repository root: %s", file.sourcePath)
		}
	}
	if err := rejectOutputCollision(archivePath, files); err != nil {
		return "", err
	}

	if options.GOOS == "windows" {
		if err := writeAtomically(archivePath, func(writer io.Writer) error {
			return writeZip(writer, files, options.Timestamp)
		}); err != nil {
			return "", err
		}
	} else {
		if err := writeAtomically(archivePath, func(writer io.Writer) error {
			return writeTarGzip(writer, files, options.Timestamp)
		}); err != nil {
			return "", err
		}
	}
	return archivePath, nil
}

func validate(options Options) error {
	switch {
	case options.BinaryPath == "":
		return errors.New("binary path is required")
	case options.Root == "":
		return errors.New("repository root is required")
	case options.OutputDir == "":
		return errors.New("output directory is required")
	case !safeIdentifier.MatchString(strings.TrimPrefix(options.Version, "v")):
		return errors.New("version contains unsupported characters")
	case !safeIdentifier.MatchString(options.GOOS):
		return errors.New("GOOS contains unsupported characters")
	case !safeIdentifier.MatchString(options.GOARCH):
		return errors.New("GOARCH contains unsupported characters")
	case options.Timestamp.IsZero():
		return errors.New("timestamp is required")
	default:
		return nil
	}
}

func rejectOutputCollision(path string, files []archiveFile) error {
	outputInfo, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect output %s: %w", path, err)
	}
	if outputInfo.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if !outputInfo.Mode().IsRegular() {
		return fmt.Errorf("output archive is not a regular file: %s", path)
	}
	for _, file := range files {
		if os.SameFile(outputInfo, file.info) {
			return fmt.Errorf("output archive aliases input %s", file.sourcePath)
		}
	}
	return nil
}

func writeAtomically(path string, write func(io.Writer) error) (returnErr error) {
	output, err := os.CreateTemp(filepath.Dir(path), ".mori-release-*")
	if err != nil {
		return fmt.Errorf("create temporary release archive: %w", err)
	}
	tempPath := output.Name()
	committed := false
	defer func() {
		if !committed {
			_ = output.Close()
			_ = os.Remove(tempPath)
		}
	}()

	if err := write(output); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return fmt.Errorf("sync release archive: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close release archive: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish release archive: %w", err)
	}
	committed = true
	return nil
}

func writeTarGzip(writer io.Writer, files []archiveFile, timestamp time.Time) error {
	gzipWriter := gzip.NewWriter(writer)
	gzipWriter.Header.ModTime = timestamp
	gzipWriter.Header.Name = ""

	tarWriter := tar.NewWriter(gzipWriter)

	for _, file := range files {
		header := &tar.Header{
			Name:       file.name,
			Mode:       file.mode,
			Size:       file.info.Size(),
			ModTime:    timestamp,
			AccessTime: timestamp,
			ChangeTime: timestamp,
			Typeflag:   tar.TypeReg,
			Format:     tar.FormatPAX,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header for %s: %w", file.name, err)
		}
		if err := copyFile(tarWriter, file); err != nil {
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close tar archive: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("close gzip archive: %w", err)
	}
	return nil
}

func writeZip(writer io.Writer, files []archiveFile, timestamp time.Time) error {
	zipWriter := zip.NewWriter(writer)

	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		header.SetMode(os.FileMode(file.mode))
		header.SetModTime(timestamp)
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("write zip header for %s: %w", file.name, err)
		}
		if err := copyFile(writer, file); err != nil {
			return err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close zip archive: %w", err)
	}
	return nil
}

func copyFile(destination io.Writer, file archiveFile) (returnErr error) {
	source, err := os.Open(file.sourcePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", file.sourcePath, err)
	}
	defer closeWithError(source, &returnErr)

	openedInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened input %s: %w", file.sourcePath, err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(file.info, openedInfo) {
		return fmt.Errorf("input changed after validation: %s", file.sourcePath)
	}
	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("copy %s: %w", file.sourcePath, err)
	}
	return nil
}

func closeWithError(closer io.Closer, returnErr *error) {
	if err := closer.Close(); err != nil && *returnErr == nil {
		*returnErr = err
	}
}
