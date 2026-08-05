package dotdrift

import (
	"context"
	"fmt"
	"os"

	"github.com/thedataflows/dotdrift/internal/paru"
)

// ParuCmd is the dotdrift mise package-plugin backend for the `paru` manager.
// These subcommands are invoked by the Lua plugin shim, not by end users
// directly. They are thin wrappers over internal/paru.
type ParuCmd struct {
	Installed ParuInstalledCmd `cmd:"" help:"Check package install status (plugin backend)"`
	Install   ParuInstallCmd   `cmd:"" help:"Install packages via paru (plugin backend)"`
}

// ParuInstalledCmd checks package status via pacman -Q and emits the line
// protocol consumed by the Lua plugin shim.
type ParuInstalledCmd struct {
	Names []string `arg:"" optional:"" help:"Package names to check"`
}

func (c *ParuInstalledCmd) Run() error {
	runner := paru.ExecRunner{}
	states := paru.PackageStatus(context.Background(), runner, c.Names)
	fmt.Print(paru.FormatStatus(states))
	return nil
}

// ParuInstallCmd installs packages via paru -S --needed --noconfirm.
type ParuInstallCmd struct {
	Names   []string `arg:"" optional:"" help:"Package names to install"`
	DryRun  bool     `name:"dry-run" help:"Print what would happen without running"`
	Update  bool     `help:"Refresh package manager metadata first"`
}

func (c *ParuInstallCmd) Run() error {
	if c.DryRun {
		fmt.Fprintf(os.Stderr, "+ paru %v\n", paru.InstallArgs(c.Names, true, c.Update))
		return nil
	}
	runner := paru.ExecRunner{}
	return paru.Install(context.Background(), runner, c.Names, false, c.Update)
}
