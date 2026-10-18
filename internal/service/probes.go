package service

import (
	"os"
	"os/exec"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
)

// Probe assembly (issue 0108, slice 2b): the status read's probes live in
// the service so every front end (CLI, TUI Status view) gets the same
// behavior. The CLI passes Elevated: true; the TUI picks per probe run.

// sudoRead executes `sudo <name> <args>` and returns stdout. A test seam so
// probe tests can assert elevation behavior without running real sudo.
var sudoRead = func(name string, args ...string) ([]byte, error) {
	return exec.Command("sudo", append([]string{name}, args...)...).Output()
}

// defaultProbes builds the status probes from the reads deps: the OS-backed
// defaults plus the package backend and mise seams, wrapped for sudo
// elevation when elevated is true.
func defaultProbes(f *facts.Facts, elevated bool, packagesFor func(string) packages.Backend, newMise func() *mise.Mise) drift.Probes {
	pr := drift.DefaultProbes()
	pr.IsInstalled = packagesFor(f.Backend).IsInstalled
	pr.ToolCurrent = mise.NewExecMise(newMise()).Current
	if elevated {
		return elevateProbes(pr) // retry elevated on permission denied (system files)
	}
	return pr
}

// elevateProbes wraps the file-access probes (Readlink, ReadFile, StatDir) so
// each retries elevated via sudo when the OS denies access. Only permission
// denied triggers the retry — other errors (not-exist, invalid path) propagate
// as-is. Already-root processes skip the wrapping entirely. Run (external
// commands like systemctl/getent) is left untouched; those commands work for
// normal users in the read-only status path.
func elevateProbes(pr drift.Probes) drift.Probes {
	if os.Geteuid() == 0 {
		return pr
	}
	if pr.Readlink != nil {
		pr.Readlink = elevateReadlink(pr.Readlink)
	}
	if pr.ReadFile != nil {
		pr.ReadFile = elevateReadFile(pr.ReadFile)
	}
	if pr.StatDir != nil {
		pr.StatDir = elevateStatDir(pr.StatDir)
	}
	if pr.ListDir != nil {
		pr.ListDir = elevateListDir(pr.ListDir)
	}
	return pr
}

// elevate wraps a probe function so it retries elevated via sudo when the OS
// denies access. The inner function runs first; on permission denied, sudo
// runs and its result is returned. When sudo itself fails, the original
// permission error is returned (the Unauthorized carrier, issue 0108).
func elevate[T any](inner func(string) (T, error), sudo func(string) (T, error)) func(string) (T, error) {
	return func(path string) (T, error) {
		result, err := inner(path)
		if err == nil || !os.IsPermission(err) {
			return result, err
		}
		sudoResult, sudoErr := sudo(path)
		if sudoErr != nil {
			return result, err
		}
		return sudoResult, nil
	}
}

func elevateReadlink(inner func(string) (string, error)) func(string) (string, error) {
	return elevate(inner, func(path string) (string, error) {
		out, err := sudoRead("readlink", path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	})
}

func elevateReadFile(inner func(string) ([]byte, error)) func(string) ([]byte, error) {
	return elevate(inner, func(path string) ([]byte, error) {
		return sudoRead("cat", path)
	})
}

func elevateStatDir(inner func(string) (bool, error)) func(string) (bool, error) {
	return elevate(inner, func(path string) (bool, error) {
		out, err := sudoRead("stat", "-c", "%F", path)
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(string(out)) == "directory", nil
	})
}

func elevateListDir(inner func(string) ([]string, error)) func(string) ([]string, error) {
	return elevateSlice(inner, func(path string) ([]string, error) {
		out, err := sudoRead("ls", "-1", path)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			return nil, nil
		}
		return lines, nil
	})
}

// elevateSlice is elevate for slice-returning probes (ListDir): retry
// elevated via sudo when the OS denies access.
func elevateSlice[T any](inner func(string) ([]T, error), sudo func(string) ([]T, error)) func(string) ([]T, error) {
	return func(path string) ([]T, error) {
		result, err := inner(path)
		if err == nil || !os.IsPermission(err) {
			return result, err
		}
		sudoResult, sudoErr := sudo(path)
		if sudoErr != nil {
			return result, err
		}
		return sudoResult, nil
	}
}
