package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Cyberlane/mori/internal/config"
)

func runInit(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)
	profile := profileReview
	writeStdout := false
	force := false
	flags.StringVar(&profile, "profile", profile, "scan profile: review, explore, or sql")
	flags.BoolVar(&writeStdout, "stdout", false, "write the deterministic configuration to standard output")
	flags.BoolVar(&force, "force", false, "replace an existing regular .mori.json file")
	flags.Usage = func() {
		fmt.Fprintln(trackedStderr, "Usage: mori init [options] [directory]")
		fmt.Fprintln(trackedStderr, "\nCreate a deterministic .mori.json with explicit profile values.")
		fmt.Fprintln(trackedStderr, "The default profile is review. Existing files are preserved unless --force is set.")
		fmt.Fprintln(trackedStderr, "\nOptions:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if trackedStderr.err != nil {
				return exitError
			}
			return exitSuccess
		}
		if trackedStderr.err != nil {
			return exitError
		}
		return exitUsage
	}
	if len(flags.Args()) > 1 {
		return usageError(stderr, "init accepts at most one directory")
	}
	if writeStdout && force {
		return usageError(stderr, "--stdout and --force cannot be used together")
	}
	if writeStdout && len(flags.Args()) != 0 {
		return usageError(stderr, "--stdout does not accept a directory")
	}
	content, err := renderProfileConfig(profile)
	if err != nil {
		return usageError(stderr, err.Error())
	}
	if writeStdout {
		if _, err := stdout.Write(content); err != nil {
			return exitError
		}
		return exitSuccess
	}

	directory := "."
	if len(flags.Args()) == 1 {
		directory = flags.Args()[0]
	}
	info, err := os.Stat(directory)
	if err != nil {
		fmt.Fprintf(stderr, "mori: init: inspect directory: %v\n", err)
		return exitError
	}
	if !info.IsDir() {
		return usageError(stderr, "init target must be a directory")
	}
	target := filepath.Join(directory, config.FileName)
	if err := writeInitialConfig(target, content, force); err != nil {
		fmt.Fprintf(stderr, "mori: init: %v\n", err)
		return exitError
	}
	if _, err := fmt.Fprintf(stdout, "Created %s\n", displayCLIPath(target)); err != nil {
		return exitError
	}
	return exitSuccess
}

func writeInitialConfig(path string, content []byte, force bool) error {
	if !force {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("%s already exists; use --force to replace it", config.FileName)
			}
			return fmt.Errorf("create config: %w", err)
		}
		if err := writeConfigFile(file, content); err != nil {
			if removeErr := os.Remove(path); removeErr != nil {
				return fmt.Errorf("%v; remove incomplete config: %w", err, removeErr)
			}
			return err
		}
		return nil
	}

	before, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeInitialConfig(path, content, false)
		}
		return fmt.Errorf("inspect config: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return errors.New("existing config is not a regular file")
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("inspect opened config: %w", err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		file.Close()
		return errors.New("config changed identity while opening")
	}
	if err := file.Truncate(0); err != nil {
		file.Close()
		return fmt.Errorf("truncate config: %w", err)
	}
	return writeConfigFile(file, content)
}

func writeConfigFile(file *os.File, content []byte) error {
	if _, err := file.Write(content); err != nil {
		file.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	return nil
}
