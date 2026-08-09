package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyberlane/mori/internal/agentskill"
)

func runSkill(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		if err := writeSkillUsage(stderr); err != nil {
			return exitError
		}
		return exitUsage
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if err := writeSkillUsage(stdout); err != nil {
			return exitError
		}
		return exitSuccess
	}
	if args[0] != "install" {
		if _, err := fmt.Fprintf(stderr, "mori: unknown skill command %q\n\n", args[0]); err != nil {
			return exitError
		}
		if err := writeSkillUsage(stderr); err != nil {
			return exitError
		}
		return exitUsage
	}

	flags := flag.NewFlagSet("skill install", flag.ContinueOnError)
	trackedStderr := &errorTrackingWriter{writer: stderr}
	flags.SetOutput(trackedStderr)
	project := flags.String("project", "", "project root; installs below .agents/skills")
	global := flags.Bool("global", false, "install below ~/.agents/skills")
	target := flags.String("target", "", "explicit parent skills directory")
	replace := flags.Bool("replace", false, "replace a different copy and preserve a backup")
	flags.Usage = func() {
		fmt.Fprintln(
			trackedStderr,
			"Usage: mori skill install (--project <path> | --global | --target <path>) [--replace]",
		)
		fmt.Fprintln(trackedStderr, "\nInstall Mori's structural-review Agent Skill.")
		fmt.Fprintln(trackedStderr, "\nOptions:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args[1:]); err != nil {
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
	if flags.NArg() != 0 {
		return usageError(stderr, "skill install does not accept positional arguments")
	}

	scopes := 0
	if strings.TrimSpace(*project) != "" {
		scopes++
	}
	if *global {
		scopes++
	}
	if strings.TrimSpace(*target) != "" {
		scopes++
	}
	if scopes != 1 {
		return usageError(
			stderr,
			"skill install requires exactly one of --project, --global, or --target",
		)
	}

	var parent string
	switch {
	case strings.TrimSpace(*project) != "":
		root, err := filepath.Abs(*project)
		if err != nil {
			return commandError(stderr, "resolve project root", err)
		}
		info, err := os.Lstat(root)
		if err != nil {
			return commandError(stderr, "inspect project root", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return commandError(
				stderr,
				"inspect project root",
				errors.New("project root must be a real directory, not a symlink"),
			)
		}
		parent = filepath.Join(root, ".agents", "skills")
	case *global:
		home, err := os.UserHomeDir()
		if err != nil {
			return commandError(stderr, "resolve home directory", err)
		}
		parent = filepath.Join(home, ".agents", "skills")
	default:
		parent = *target
	}

	result, err := agentskill.Install(parent, *replace)
	if err != nil {
		if errors.Is(err, agentskill.ErrDifferent) {
			return commandError(
				stderr,
				"install skill",
				fmt.Errorf("%w; review it or rerun with --replace", err),
			)
		}
		return commandError(stderr, "install skill", err)
	}
	switch result.Status {
	case agentskill.StatusInstalled:
		_, err = fmt.Fprintf(stdout, "Installed %s at %s\n", agentskill.Name, result.Path)
	case agentskill.StatusCurrent:
		_, err = fmt.Fprintf(stdout, "%s is current at %s\n", agentskill.Name, result.Path)
	case agentskill.StatusReplaced:
		_, err = fmt.Fprintf(
			stdout,
			"Updated %s at %s\nPrevious copy: %s\n",
			agentskill.Name,
			result.Path,
			result.BackupPath,
		)
	default:
		return commandError(stderr, "install skill", errors.New("unknown installation status"))
	}
	if err != nil {
		return exitError
	}
	return exitSuccess
}

func commandError(stderr io.Writer, operation string, err error) int {
	_, _ = fmt.Fprintf(stderr, "mori: %s: %v\n", operation, err)
	return exitError
}

func writeSkillUsage(writer io.Writer) error {
	_, err := fmt.Fprint(
		writer,
		"Usage:\n",
		"  mori skill install (--project <path> | --global | --target <path>) [--replace]\n",
	)
	return err
}
