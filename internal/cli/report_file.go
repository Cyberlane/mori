package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
	"github.com/Cyberlane/mori/internal/report"
)

func writeCompleteJSONReport(path string, value model.Report) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("report output path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve report output: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("report output must be a regular file, not a symlink")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect report output: %w", err)
	}
	directory := filepath.Dir(absolute)
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("inspect report output directory: %w", err)
	}
	if !directoryInfo.IsDir() {
		return errors.New("report output parent is not a directory")
	}

	temporary, err := os.CreateTemp(directory, ".mori-report-*.json")
	if err != nil {
		return fmt.Errorf("create temporary report: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect temporary report: %w", err)
	}
	if err := report.JSON(temporary, value); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode report: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync report: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close report: %w", err)
	}
	if err := os.Rename(temporaryPath, absolute); err != nil {
		return fmt.Errorf("publish report: %w", err)
	}
	committed = true
	if err := syncConfigDirectory(directory); err != nil {
		return fmt.Errorf("sync report directory: %w", err)
	}
	return nil
}
