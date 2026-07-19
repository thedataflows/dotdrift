package packages

import (
	"context"
	"fmt"
	"os/exec"
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
	if _, err := a.Runner.Run(ctx, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt update: %w", err)
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if _, err := a.Runner.Run(ctx, "apt-get", args...); err != nil {
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
	if _, err := a.Runner.Run(ctx, "apt-get", args...); err != nil {
		return fmt.Errorf("apt remove %v: %w", pkgs, err)
	}
	return nil
}

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
	if _, err := d.Runner.Run(ctx, "dnf", args...); err != nil {
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
	if _, err := d.Runner.Run(ctx, "dnf", args...); err != nil {
		return fmt.Errorf("dnf remove %v: %w", pkgs, err)
	}
	return nil
}

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
