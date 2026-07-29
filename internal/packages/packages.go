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

	"github.com/thedataflows/dotdrift/internal/apply"
	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Backend performs package operations.
type Backend interface {
	// Present installs the given packages if not already present.
	Present(ctx context.Context, pkgs []string) error
	// Absent removes the given packages.
	Absent(ctx context.Context, pkgs []string) error
	// IsInstalled reports whether a package is already installed.
	IsInstalled(ctx context.Context, pkg string) (bool, error)
}

// Runner runs a command and returns stdout; cancelling ctx kills the child process.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner is the real command runner. Run always captures stdout and
// discards it (probe path). Verbose streams child stdout/stderr live to
// Out/Err on the streaming entry point (RunStream) used by install/remove;
// probes via Run stay captured either way.
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
// Out/Err when Verbose is set, returning no captured output (the terminal
// already shows it). With Verbose unset it is byte-identical to Run.
func (e ExecRunner) RunStream(ctx context.Context, name string, args ...string) (string, error) {
	if !e.Verbose {
		return e.Run(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	out, errW := e.Out, e.Err
	if out == nil {
		out = os.Stdout
	}
	if errW == nil {
		errW = os.Stderr
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

// Step is the apply pipeline step for packages.
type Step struct {
	backend Backend
	plan    *resolve.Plan
}

var _ apply.Step = (*Step)(nil)

// NewStep creates a package step bound to a plan and backend.
func NewStep(backend Backend, plan *resolve.Plan) *Step {
	return &Step{backend: backend, plan: plan}
}

// Name returns the step name.
func (s *Step) Name() string { return "packages" }

// Run applies the package backend, removing absent packages first, then installing present ones.
func (s *Step) Run(ctx context.Context) error {
	if s.plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if len(s.plan.Packages.Remove) > 0 {
		if err := s.backend.Absent(ctx, s.plan.Packages.Remove); err != nil {
			return fmt.Errorf("remove packages: %w", err)
		}
	}
	if len(s.plan.Packages.Install) > 0 {
		if err := s.backend.Present(ctx, s.plan.Packages.Install); err != nil {
			return fmt.Errorf("install packages: %w", err)
		}
	}
	return nil
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
