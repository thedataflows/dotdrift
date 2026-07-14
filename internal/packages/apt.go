package packages

import (
	"fmt"
)

// Apt is the Debian/Ubuntu backend skeleton.
type Apt struct {
	Runner Runner
}

// NewApt returns an Apt backend using the real command runner.
func NewApt() *Apt {
	return &Apt{Runner: ExecRunner{}}
}

// Present installs packages idempotently.
func (a *Apt) Present(pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if _, err := a.Runner.Run("apt-get", args...); err != nil {
		return fmt.Errorf("apt install %v: %w", pkgs, err)
	}
	return nil
}

// Absent removes packages.
func (a *Apt) Absent(pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, pkgs...)
	if _, err := a.Runner.Run("apt-get", args...); err != nil {
		return fmt.Errorf("apt remove %v: %w", pkgs, err)
	}
	return nil
}

// IsInstalled checks if a package is installed via dpkg.
func (a *Apt) IsInstalled(pkg string) (bool, error) {
	_, err := a.Runner.Run("dpkg", "-l", pkg)
	if err == nil {
		return true, nil
	}
	return false, nil
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
func (d *Dnf) Present(pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if _, err := d.Runner.Run("dnf", args...); err != nil {
		return fmt.Errorf("dnf install %v: %w", pkgs, err)
	}
	return nil
}

// Absent removes packages.
func (d *Dnf) Absent(pkgs []string) error {
	pkgs = uniqueSorted(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, pkgs...)
	if _, err := d.Runner.Run("dnf", args...); err != nil {
		return fmt.Errorf("dnf remove %v: %w", pkgs, err)
	}
	return nil
}

// IsInstalled checks if a package is installed via rpm.
func (d *Dnf) IsInstalled(pkg string) (bool, error) {
	_, err := d.Runner.Run("rpm", "-q", pkg)
	if err == nil {
		return true, nil
	}
	return false, nil
}
