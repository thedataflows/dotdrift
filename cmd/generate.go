package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The generate command group materializes self-contained mounts/smb
// modules through the service writes area (T-tui-writes), which calls
// internal/generate.WriteModule — the one assembly and write path, shared
// with the TUI's generate flows (issue 0066: interactivity lives in
// `dotdrift tui`; each subcommand here assembles its generate.Input from
// the input flags, and the required flags are validated loudly).

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
	Name        string    `help:"Mount name (required)"`
	Source      string    `help:"Mount source, e.g. UUID=<uuid> or server:/export (required)"`
	Destination string    `help:"Mount destination path (required)"`
	Type        string    `help:"Filesystem type; the registry preset applies when --option is omitted (required)"`
	Option      []string  `help:"Mount option (repeatable; overrides the registry preset for --type)"`
	StartAt     string    `name:"startat" help:"OnCalendar expression; adds a .timer/.service pair"`
	State       string    `help:"Mount state: enabled or disabled (default enabled)"`
	ListVolumes bool      `name:"list-volumes" help:"Print detected volumes and exit"`
	Out         io.Writer `kong:"-"`
}

// Run implements the mounts generator: the volume table, or CLI-mode
// assembly + service write.
func (c *GenerateMountsCmd) Run() error {
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	area := service.NewWritesArea(service.WritesDeps{Detect: detectFacts})

	sel, err := area.GenerateSelection(generate.Selection{
		Layer: c.Layer, ModuleID: c.Module, Hostname: c.Hostname, Username: c.Username,
	})
	if err != nil {
		return err
	}

	if c.ListVolumes {
		vols, err := area.Volumes(c.Profile, sel)
		if err != nil {
			return err
		}
		return printGenerateVolumes(out, vols)
	}

	if err := c.validate(); err != nil {
		return err
	}

	uid, gid, _, err := generate.InvokingUser()
	if err != nil {
		return err
	}
	input := generate.MountsInput(map[string]profile.MountSpec{
		c.Name: {
			Source:      c.Source,
			Destination: c.Destination,
			Type:        c.Type,
			Options:     c.Option,
			StartAt:     c.StartAt,
			State:       c.State,
		},
	}, uid, gid)
	if err := area.WriteGenerate(c.Profile, sel, input, out); err != nil {
		return fmt.Errorf("generate mounts: %w", err)
	}
	return nil
}

// validate enforces the required flags loudly, naming every missing
// flag.
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
	Group    string    `help:"Samba group (default \"smb\")"`
	Users    []string  `name:"user" help:"Samba user (repeatable; default: the invoking user)"`
	Avahi    *bool     `negatable:"" help:"Avahi service discovery (default on; --no-avahi records an explicit off)"`
	Shares   []string  `name:"share" help:"Share as name=path (repeatable; at least one required)"`
	Writable *bool     `negatable:"" help:"Shares writable (default on)"`
	Readonly bool      `help:"Shares read-only: sets writable=false"`
	Public   bool      `help:"Shares public (guest access)"`
	Out      io.Writer `kong:"-"`
}

// Run implements the smb generator.
func (c *GenerateSmbCmd) Run() error {
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	area := service.NewWritesArea(service.WritesDeps{Detect: detectFacts})

	sel, err := area.GenerateSelection(generate.Selection{
		Layer: c.Layer, ModuleID: c.Module, Hostname: c.Hostname, Username: c.Username,
	})
	if err != nil {
		return err
	}

	shares, err := generate.ParseShareFlags(c.Shares, generate.ResolveWritable(c.Writable, c.Readonly), c.Public)
	if err != nil {
		return err
	}

	uid, gid, username, err := generate.InvokingUser()
	if err != nil {
		return err
	}
	input := generate.SmbInput(c.Group, c.Users, c.Avahi, shares, username, uid, gid)
	if err := area.WriteGenerate(c.Profile, sel, input, out); err != nil {
		return fmt.Errorf("generate smb: %w", err)
	}
	return nil
}
