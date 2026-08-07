// Package packages provides package backend operations.
package packages

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/executil"
)

// Backend performs package operations.
type Backend interface {
	// Present installs the given packages if not already present.
	Present(ctx context.Context, pkgs []string) error
	// Absent removes the given packages.
	Absent(ctx context.Context, pkgs []string) error
	// IsInstalled reports whether a package is already installed.
	IsInstalled(ctx context.Context, pkg string) (bool, error)
	// DirectDeps returns the names of pkg's direct runtime dependencies.
	DirectDeps(ctx context.Context, pkg string) ([]string, error)
}

// Runner runs a command and returns stdout; cancelling ctx kills the child process.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner is the real command runner. Run always captures stdout and
// discards it (probe path). Verbose streams child stdout/stderr live to
// Out/Err on the streaming entry point (RunStream) used by install/remove,
// echoing each command line set -x-style ("+ argv") to Err before it runs;
// probes via Run stay captured and unechoed either way.
type ExecRunner struct {
	Verbose bool
	// Out/Err are the Verbose streaming destinations; nil defaults to
	// os.Stdout/os.Stderr.
	Out io.Writer
	Err io.Writer
}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// RunStream runs like Run but streams the child's stdout/stderr live to
// Out/Err when Verbose is set OR both destinations are terminals (interactive
// apply), wiring the child's fds straight to them so it keeps its own color
// output. Under Verbose the command line is echoed set -x-style ("+ argv") to
// Err before execution; the interactive default omits that trace. Streaming
// returns no captured output (the terminal already shows it); a non-verbose,
// piped run captures via Run instead.
func (e ExecRunner) RunStream(ctx context.Context, name string, args ...string) (string, error) {
	out, errW := e.Out, e.Err
	if out == nil {
		out = os.Stdout
	}
	if errW == nil {
		errW = os.Stderr
	}
	if !executil.StreamLive(e.Verbose, out, errW) {
		return e.Run(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if e.Verbose {
		executil.EchoCommand(errW, append([]string{name}, args...))
	}
	cmd.Stdout = out
	cmd.Stderr = errW
	return "", cmd.Run()
}

// streamRunner is ExecRunner's verbose-capable surface. Fakes implement only
// Runner, so they stay on the captured path untouched.
type streamRunner interface {
	RunStream(ctx context.Context, name string, args ...string) (string, error)
}

// runMutating runs an install/remove command: through the streaming surface
// when the runner offers one, otherwise the captured Run path. Probes never
// come through here — they call Runner.Run directly.
func runMutating(ctx context.Context, r Runner, name string, args ...string) (string, error) {
	if sr, ok := r.(streamRunner); ok {
		return sr.RunStream(ctx, name, args...)
	}
	return r.Run(ctx, name, args...)
}

// setVerboseOn flips Verbose when the backend's runner is the real
// ExecRunner; injected fakes are left untouched.
func setVerboseOn(r *Runner, v bool) {
	if er, ok := (*r).(ExecRunner); ok {
		er.Verbose = v
		*r = er
	}
}

// Paru is the Arch/CachyOS backend.
type Paru struct {
	Runner Runner
}

// NewParu returns a Paru backend using the real command runner.
func NewParu() *Paru {
	return &Paru{Runner: ExecRunner{}}
}

// Present installs packages idempotently.
func (p *Paru) Present(ctx context.Context, pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"-S", "--needed", "--noconfirm"}, pkgs...)
	_, err := runMutating(ctx, p.Runner, "paru", args...)
	if err != nil {
		return fmt.Errorf("paru install %v: %w", pkgs, err)
	}
	return nil
}

// Absent removes packages.
func (p *Paru) Absent(ctx context.Context, pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"-R", "--noconfirm"}, pkgs...)
	_, err := runMutating(ctx, p.Runner, "paru", args...)
	if err != nil {
		return fmt.Errorf("paru remove %v: %w", pkgs, err)
	}
	return nil
}

// SetVerbose toggles live output streaming when the backend runs on the real
// ExecRunner.
func (p *Paru) SetVerbose(v bool) { setVerboseOn(&p.Runner, v) }

// IsInstalled checks if a package is installed via pacman.
func (p *Paru) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	_, err := p.Runner.Run(ctx, "pacman", "-Q", pkg)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func uniqueSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// AutoBackend resolves the backend string from the runtime environment.
// It is a variable so tests can substitute it.
var AutoBackend = func() (string, error) {
	f, err := detect.Detect()
	if err != nil {
		return "", err
	}
	return f.Backend, nil
}

// For selects a backend for the given facts.
// It returns a Paru backend for Arch-family distros, Apt for Debian/Ubuntu,
// Dnf for Fedora, and resolves from os-release for "auto".
func For(backend string) Backend {
	switch strings.ToLower(backend) {
	case "paru", "arch", "cachyos", "manjaro":
		return NewParu()
	case "apt", "debian", "ubuntu":
		return NewApt()
	case "dnf", "fedora":
		return NewDnf()
	case "auto":
		if b, err := AutoBackend(); err == nil {
			return For(b)
		}
		return &noop{backend: backend}
	default:
		return &noop{backend: backend}
	}
}

// noop fails loudly so an unsupported distro never looks like a successful apply.
type noop struct {
	backend string
}

func (n *noop) err() error {
	return fmt.Errorf("no supported package backend for distro %q", n.backend)
}

func (n *noop) Present(context.Context, []string) error { return n.err() }
func (n *noop) Absent(context.Context, []string) error  { return n.err() }
func (n *noop) IsInstalled(context.Context, string) (bool, error) {
	return false, n.err()
}

func (n *noop) DirectDeps(context.Context, string) ([]string, error) { return nil, n.err() }
