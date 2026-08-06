package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/tui"
)

// The generate command group materializes self-contained mounts/smb
// modules via internal/generate.WriteModule. Two modes per subcommand:
//
//   - CLI mode: any input flag is present (or --no-tui was given); the
//     required input flags are validated loudly, a generate.Input is
//     assembled, and WriteModule runs.
//   - Wizard mode: no input flags and a terminal (or --tui, which forces
//     the wizard even with input flags); control passes to the TUI
//     wizard through the runWizard seam.
//
// Test seams (package-level vars, same pattern as detectFacts in
// apply.go; the isTTY probe lives in generate_support.go):

// runWizard hands a generate subcommand to the interactive TUI wizard
// (internal/tui). The invocation carries the parsed subcommand so the
// wizard pre-fills from any already-parsed input flags; the wizard owns
// everything after invocation (including generate.WriteModule).
var runWizard = func(inv wizardInvocation) error {
	switch {
	case inv.Mounts != nil:
		c := inv.Mounts
		return tui.RunMountsWizard(context.Background(), tui.MountsParams{
			Profile:     c.Profile,
			Layer:       c.Layer,
			ModuleID:    c.Module,
			Hostname:    c.Hostname,
			Username:    c.Username,
			Name:        c.Name,
			Source:      c.Source,
			Destination: c.Destination,
			Type:        c.Type,
			Options:     c.Option,
			StartAt:     c.StartAt,
			State:       c.State,
		})
	case inv.Smb != nil:
		c := inv.Smb
		return tui.RunSmbWizard(context.Background(), tui.SmbParams{
			Profile:  c.Profile,
			Layer:    c.Layer,
			ModuleID: c.Module,
			Hostname: c.Hostname,
			Username: c.Username,
			Group:    c.Group,
			Users:    c.Users,
			Avahi:    c.Avahi,
			Shares:   c.Shares,
			Writable: c.Writable,
			Readonly: c.Readonly,
			Public:   c.Public,
		})
	default:
		return errors.New("generate: empty wizard invocation")
	}
}

// wizardInvocation carries the invoking generate subcommand to the
// wizard seam: exactly one field is non-nil.
type wizardInvocation struct {
	Mounts *GenerateMountsCmd
	Smb    *GenerateSmbCmd
}

// GenerateCmd groups the module generators.
type GenerateCmd struct {
	Mounts GenerateMountsCmd `cmd:"" help:"Generate a mounts module (systemd units)"`
	Smb    GenerateSmbCmd    `cmd:"" help:"Generate an smb module (samba shares)"`
}

// GenerateMountsCmd generates a mounts module: systemd .mount units
// (plus .service/.timer pairs for --startat) and the module.toml scope,
// [mounts] section, packages, and dotfile entries that place them.
type GenerateMountsCmd struct {
	Profile     string    `help:"Path to profile directory" type:"existingdir" default:"."`
	Layer       string    `help:"Target layer" enum:"base,host,user" default:"base"`
	Module      string    `help:"Module id" default:"mounts"`
	Hostname    string    `help:"Hostname for --layer host (default: detected)"`
	Username    string    `help:"Username for --layer user (default: detected)"`
	Name        string    `help:"Mount name (input flag)"`
	Source      string    `help:"Mount source, e.g. UUID=<uuid> or server:/export (input flag)"`
	Destination string    `help:"Mount destination path (input flag)"`
	Type        string    `help:"Filesystem type; the registry preset applies when --option is omitted (input flag)"`
	Option      []string  `help:"Mount option (repeatable; overrides the registry preset for --type)"`
	StartAt     string    `name:"startat" help:"OnCalendar expression; adds a .timer/.service pair (input flag)"`
	State       string    `help:"Mount state: enabled or disabled (default enabled)"`
	ListVolumes bool      `name:"list-volumes" help:"Print detected volumes and exit"`
	TUI         *bool     `negatable:"" help:"Force the interactive wizard (--no-tui forces CLI mode)"`
	Out         io.Writer `kong:"-"`
}

// hasInput reports whether any input flag was given (State carries no
// kong default precisely so an explicit --state is detectable here).
func (c *GenerateMountsCmd) hasInput() bool {
	return c.Name != "" || c.Source != "" || c.Destination != "" || c.Type != "" ||
		len(c.Option) > 0 || c.StartAt != "" || c.State != ""
}

// Run implements the mounts generator: mode selection, then either the
// volume table, the wizard, or CLI-mode assembly + WriteModule.
func (c *GenerateMountsCmd) Run() error {
	out := c.Out
	if out == nil {
		out = os.Stdout
	}

	sel, err := generateSelection(generate.Selection{
		Layer: c.Layer, ModuleID: c.Module, Hostname: c.Hostname, Username: c.Username,
	})
	if err != nil {
		return err
	}

	if c.ListVolumes {
		return printGenerateVolumes(out, c.Profile, sel)
	}

	mode, err := selectGenerateMode(generateModeQuery{
		Sub:      "mounts",
		TUI:      c.TUI,
		HasInput: c.hasInput(),
		Required: "--name, --source, --destination, --type",
	})
	if err != nil {
		return err
	}
	if mode == generateModeWizard {
		return runWizard(wizardInvocation{Mounts: c})
	}

	if err := c.validate(); err != nil {
		return err
	}

	uid, gid, _, err := tui.InvokingUser()
	if err != nil {
		return err
	}
	input := tui.MountsInput(map[string]profile.MountSpec{
		c.Name: {
			Source:      c.Source,
			Destination: c.Destination,
			Type:        c.Type,
			Options:     c.Option,
			StartAt:     c.StartAt,
			State:       c.State,
		},
	}, uid, gid)
	if err := generate.WriteModule(c.Profile, sel, input); err != nil {
		return fmt.Errorf("generate mounts: %w", err)
	}
	return tui.PrintSummary(out, c.Profile, sel)
}

// validate enforces the CLI-mode required flags loudly, naming every
// missing flag.
func (c *GenerateMountsCmd) validate() error {
	var missing []string
	if c.Name == "" {
		missing = append(missing, "--name")
	}
	if c.Source == "" {
		missing = append(missing, "--source")
	}
	if c.Destination == "" {
		missing = append(missing, "--destination")
	}
	if c.Type == "" {
		missing = append(missing, "--type")
	}
	if len(missing) > 0 {
		return fmt.Errorf("generate mounts: missing required flag(s): %s", strings.Join(missing, ", "))
	}
	switch c.State {
	case "", "enabled", "disabled":
	default:
		return fmt.Errorf("generate mounts: --state must be \"enabled\" or \"disabled\", got %q", c.State)
	}
	return nil
}

// GenerateSmbCmd generates an smb module: shares.conf, a one-time
// smb.conf seed, and the module.toml scope, [smb] section, packages, and
// dotfile entries that place them.
type GenerateSmbCmd struct {
	Profile  string    `help:"Path to profile directory" type:"existingdir" default:"."`
	Layer    string    `help:"Target layer" enum:"base,host,user" default:"base"`
	Module   string    `help:"Module id" default:"smb"`
	Hostname string    `help:"Hostname for --layer host (default: detected)"`
	Username string    `help:"Username for --layer user (default: detected)"`
	Group    string    `help:"Samba group (default \"smb\"; input flag)"`
	Users    []string  `name:"user" help:"Samba user (repeatable; default: the invoking user)"`
	Avahi    *bool     `negatable:"" help:"Avahi service discovery (default on; --no-avahi records an explicit off)"`
	Shares   []string  `name:"share" help:"Share as name=path (repeatable; at least one required in CLI mode)"`
	Writable *bool     `negatable:"" help:"Shares writable (default on; input flag)"`
	Readonly bool      `help:"Shares read-only: sets writable=false"`
	Public   bool      `help:"Shares public (guest access; input flag)"`
	TUI      *bool     `negatable:"" help:"Force the interactive wizard (--no-tui forces CLI mode)"`
	Out      io.Writer `kong:"-"`
}

// hasInput reports whether any input flag was given. Avahi/Writable are
// *bool so an explicit flag is distinguishable from the default.
func (c *GenerateSmbCmd) hasInput() bool {
	return c.Group != "" || len(c.Users) > 0 || c.Avahi != nil || len(c.Shares) > 0 ||
		c.Writable != nil || c.Readonly || c.Public
}

// Run implements the smb generator.
func (c *GenerateSmbCmd) Run() error {
	out := c.Out
	if out == nil {
		out = os.Stdout
	}

	sel, err := generateSelection(generate.Selection{
		Layer: c.Layer, ModuleID: c.Module, Hostname: c.Hostname, Username: c.Username,
	})
	if err != nil {
		return err
	}

	mode, err := selectGenerateMode(generateModeQuery{
		Sub:      "smb",
		TUI:      c.TUI,
		HasInput: c.hasInput(),
		Required: "--share name=path (at least one)",
	})
	if err != nil {
		return err
	}
	if mode == generateModeWizard {
		return runWizard(wizardInvocation{Smb: c})
	}

	shares, err := tui.ParseShareFlags(c.Shares, tui.ResolveWritable(c.Writable, c.Readonly), c.Public)
	if err != nil {
		return err
	}

	uid, gid, username, err := tui.InvokingUser()
	if err != nil {
		return err
	}
	input := tui.SmbInput(c.Group, c.Users, c.Avahi, shares, username, uid, gid)
	if err := generate.WriteModule(c.Profile, sel, input); err != nil {
		return fmt.Errorf("generate smb: %w", err)
	}
	return tui.PrintSummary(out, c.Profile, sel)
}
