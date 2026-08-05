// Package paru implements the dotdrift mise package-plugin backend for the
// `paru` manager (pacman + AUR). The dotdrift paru subcommand exposes two
// operations — installed (status check) and install — that the Lua plugin
// shim delegates to. This keeps all paru invocation logic in Go.
package paru

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes name with args, returning trimmed stdout on success or the
// combined output on failure (so callers surface the tool's own diagnostics).
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return strings.TrimSpace(string(out)), nil
}
// Runner executes one command and returns its stdout (trimmed) on success or
// the combined output on failure.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner runs commands via exec.CommandContext.
type ExecRunner struct{}

// State is one element of the installed-status response.
type State struct {
	Name    string
	Installed bool
	Version  string
}

// PackageStatus checks each package via `pacman -Q <name>`. The query is
// read-only and never elevates. A package present in pacman's local database
// (covering both repo and AUR packages) is "installed"; anything else is
// "missing". Errors from individual queries are tolerated (treated as
// missing) so one bad name never blocks the batch.
func PackageStatus(ctx context.Context, runner Runner, names []string) []State {
	states := make([]State, len(names))
	for i, name := range names {
		out, err := runner.Run(ctx, "pacman", "-Q", name)
		if err != nil {
			states[i] = State{Name: name}
			continue
		}
		// pacman -Q output: "name\tversion"
		fields := strings.Fields(out)
		ver := ""
		if len(fields) >= 2 {
			ver = fields[1]
		}
		states[i] = State{Name: name, Installed: true, Version: ver}
	}
	return states
}

// FormatStatus emits the line-based protocol consumed by the Lua plugin shim:
// one line per package, tab-separated: name<TAB>installed|missing<TAB>version.
func FormatStatus(states []State) string {
	var b strings.Builder
	for _, s := range states {
		if s.Installed {
			fmt.Fprintf(&b, "%s\tinstalled\t%s\n", s.Name, s.Version)
		} else {
			fmt.Fprintf(&b, "%s\tmissing\n", s.Name)
		}
	}
	return b.String()
}

// InstallArgs builds the argv for installing packages via paru.
func InstallArgs(names []string, dryRun, update bool) []string {
	args := []string{"-S", "--needed", "--noconfirm"}
	if update {
		args = append(args, "-y")
	}
	args = append(args, names...)
	return args
}

// Install runs `paru -S --needed --noconfirm <names>`. paru self-elevates
// (calls sudo pacman internally); this function invokes no sudo itself.
func Install(ctx context.Context, runner Runner, names []string, dryRun, update bool) error {
	if len(names) == 0 {
		return nil
	}
	if dryRun {
		return nil
	}
	_, err := runner.Run(ctx, "paru", InstallArgs(names, false, update)...)
	return err
}
