// Package packages provides package backend operations.
package packages

import (
	"context"
	"fmt"
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
	Present(pkgs []string) error
	// Absent removes the given packages.
	Absent(pkgs []string) error
	// IsInstalled reports whether a package is already installed.
	IsInstalled(pkg string) (bool, error)
}

// Runner runs a command and returns stdout.
type Runner interface {
	Run(name string, args ...string) (string, error)
}

// ExecRunner is the real command runner.
type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
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
func (p *Paru) Present(pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"-S", "--needed", "--noconfirm"}, pkgs...)
	_, err := p.Runner.Run("paru", args...)
	if err != nil {
		return fmt.Errorf("paru install %v: %w", pkgs, err)
	}
	return nil
}

// Absent removes packages.
func (p *Paru) Absent(pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"-R", "--noconfirm"}, pkgs...)
	_, err := p.Runner.Run("paru", args...)
	if err != nil {
		return fmt.Errorf("paru remove %v: %w", pkgs, err)
	}
	return nil
}

// IsInstalled checks if a package is installed via pacman.
func (p *Paru) IsInstalled(pkg string) (bool, error) {
	_, err := p.Runner.Run("pacman", "-Q", pkg)
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

// Run applies the package backend.
func (s *Step) Run(ctx context.Context) error {
	if s.plan == nil {
		return fmt.Errorf("plan is nil")
	}
	return s.backend.Present(s.plan.Packages.Install)
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
		return &noop{}
	default:
		return &noop{}
	}
}

type noop struct{}

func (noop) Present([]string) error         { return nil }
func (noop) Absent([]string) error           { return nil }
func (noop) IsInstalled(string) (bool, error) { return false, nil }
