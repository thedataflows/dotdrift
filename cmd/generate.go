package dotdrift

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
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

// runWizard is the interactive generate wizard seam owned by T13.
//
// T13: implement the wizard and reassign this var (e.g. from the tui
// package's wiring) — the call sites in Run need no further changes.
// The invocation carries the parsed subcommand so the wizard can
// pre-fill from any already-parsed input flags; the wizard owns
// everything after invocation (including generate.WriteModule).
// Until then the stub fails loudly.
var runWizard = func(wizardInvocation) error {
	return errors.New("generate: wizard not yet available (T13); pass input flags for CLI mode or --no-tui")
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

	uid, gid, _, err := invokingUser()
	if err != nil {
		return err
	}
	state := c.State
	if state == "" {
		state = "enabled"
	}
	input := generate.Input{
		Mounts: map[string]profile.MountSpec{
			c.Name: {
				Source:      c.Source,
				Destination: c.Destination,
				Type:        c.Type,
				Options:     c.Option,
				StartAt:     c.StartAt,
				State:       state,
			},
		},
		UID: uid,
		GID: gid,
	}
	if err := generate.WriteModule(c.Profile, sel, input); err != nil {
		return fmt.Errorf("generate mounts: %w", err)
	}
	return printGenerateSummary(out, c.Profile, sel)
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

	shares, err := c.parseShares()
	if err != nil {
		return err
	}

	uid, gid, username, err := invokingUser()
	if err != nil {
		return err
	}
	users := c.Users
	if len(users) == 0 {
		users = []string{username}
	}
	group := c.Group
	if group == "" {
		group = "smb"
	}
	// Writable defaults on; --no-writable stores an explicit false and
	// --readonly is its shorthand (and wins when both are given).
	writable := true
	if c.Writable != nil {
		writable = *c.Writable
	}
	if c.Readonly {
		writable = false
	}
	for name, share := range shares {
		share.Writable = writable
		share.Public = c.Public
		shares[name] = share
	}

	input := generate.Input{
		Smb: &profile.SmbSpec{
			Group:  group,
			Users:  users,
			Avahi:  c.Avahi, // nil keeps the default-on semantics; --no-avahi records explicit false
			Shares: shares,
		},
		UID: uid,
		GID: gid,
	}
	if err := generate.WriteModule(c.Profile, sel, input); err != nil {
		return fmt.Errorf("generate smb: %w", err)
	}
	return printGenerateSummary(out, c.Profile, sel)
}

// parseShares validates and decodes the --share name=path values.
func (c *GenerateSmbCmd) parseShares() (map[string]profile.ShareSpec, error) {
	if len(c.Shares) == 0 {
		return nil, fmt.Errorf("generate smb: missing required flag: --share name=path (at least one share)")
	}
	shares := make(map[string]profile.ShareSpec, len(c.Shares))
	for _, s := range c.Shares {
		name, path, ok := strings.Cut(s, "=")
		if !ok || name == "" || path == "" {
			return nil, fmt.Errorf("generate smb: invalid --share %q: want name=path with both name and path non-empty", s)
		}
		shares[name] = profile.ShareSpec{Path: path}
	}
	return shares, nil
}
