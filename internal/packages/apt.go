package packages

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Apt is the Debian/Ubuntu backend skeleton.
type Apt struct {
	Runner Runner
}

// NewApt returns an Apt backend using the real command runner.
func NewApt() *Apt {
	return &Apt{Runner: ExecRunner{}}
}

// Present installs packages idempotently. Unlike dnf (which refreshes
// repository metadata as part of install), apt needs an explicit index
// refresh first: on a fresh machine/container the index is empty or stale
// and `apt-get install` fails with "Unable to locate package".
func (a *Apt) Present(ctx context.Context, pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	if _, err := runMutating(ctx, a.Runner, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if _, err := runMutating(ctx, a.Runner, "apt-get", args...); err != nil {
		return fmt.Errorf("apt install %v: %w", pkgs, err)
	}
	return nil
}

// Absent removes packages.
func (a *Apt) Absent(ctx context.Context, pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, pkgs...)
	if _, err := runMutating(ctx, a.Runner, "apt-get", args...); err != nil {
		return fmt.Errorf("apt remove %v: %w", pkgs, err)
	}
	return nil
}

// SetVerbose toggles live output streaming when the backend runs on the real
// ExecRunner.
func (a *Apt) SetVerbose(v bool) { setVerboseOn(&a.Runner, v) }

// IsInstalled checks if a package is installed via dpkg.
func (a *Apt) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	_, err := a.Runner.Run(ctx, "dpkg", "-l", pkg)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// Installed lists every installed package name via dpkg-query.
func (a *Apt) Installed(ctx context.Context) ([]string, error) {
	out, err := a.Runner.Run(ctx, "dpkg-query", "-W", "-f", "${Package}\n")
	if err != nil {
		return nil, fmt.Errorf("dpkg list installed: %w", err)
	}
	return strings.Fields(out), nil
}

// Dnf is the Fedora/RHEL backend skeleton.
type Dnf struct {
	Runner Runner
}

// NewDnf returns a Dnf backend using the real command runner.
func NewDnf() *Dnf {
	return &Dnf{Runner: ExecRunner{}}
}

// Present installs packages idempotently.
func (d *Dnf) Present(ctx context.Context, pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if _, err := runMutating(ctx, d.Runner, "dnf", args...); err != nil {
		return fmt.Errorf("dnf install %v: %w", pkgs, err)
	}
	return nil
}

// Absent removes packages.
func (d *Dnf) Absent(ctx context.Context, pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, pkgs...)
	if _, err := runMutating(ctx, d.Runner, "dnf", args...); err != nil {
		return fmt.Errorf("dnf remove %v: %w", pkgs, err)
	}
	return nil
}

// SetVerbose toggles live output streaming when the backend runs on the real
// ExecRunner.
func (d *Dnf) SetVerbose(v bool) { setVerboseOn(&d.Runner, v) }

// IsInstalled checks if a package is installed via rpm.
func (d *Dnf) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	_, err := d.Runner.Run(ctx, "rpm", "-q", pkg)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// Installed lists every installed package name via rpm.
func (d *Dnf) Installed(ctx context.Context) ([]string, error) {
	out, err := d.Runner.Run(ctx, "rpm", "-qa", "--qf", "%{NAME}\n")
	if err != nil {
		return nil, fmt.Errorf("rpm list installed: %w", err)
	}
	return strings.Fields(out), nil
}
