// Package tui implements the interactive `dotdrift generate` wizard.
//
// The wizard is split in two layers:
//
//   - a pure spec-builder state machine (this package's MountsWizard /
//     SmbWizard and the choice types) that accumulates user choices and
//     assembles generate.Input through the SAME assembly helpers the CLI
//     mode uses (MountsInput / SmbInput / ParseShareFlags). It is
//     hermetically testable: feed choices, get a generate.Input, write
//     with Write — no TTY involved. CLI mode and wizard mode therefore
//     produce byte-identical module trees for the same logical inputs.
//   - a thin huh presentation layer (wizard_mounts.go, wizard_smb.go)
//     that gathers the same choices interactively and feeds the state
//     machine. lipgloss renders the minimal tab chrome (mounts | smb).
//
// Write semantics: the mounts wizard accumulates every mount and calls
// generate.WriteModule ONCE at the end (WriteModule replaces [mounts]
// wholesale, so a mid-loop write would wipe earlier iterations). The
// smb wizard writes once after the shares loop. Aborting (ctrl+c/esc)
// before the final confirmation discards all unwritten choices.
package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Network-source validation patterns: nfs-style "host:/export" and
// cifs-style "//host/share".
var (
	nfsSourceRe  = regexp.MustCompile(`^[^:/]+:/.+`)
	cifsSourceRe = regexp.MustCompile(`^//[^/]+/.+`)
)

// ValidateNetworkSource accepts nfs-style "host:/export" and cifs-style
// "//host/share" sources; anything else is rejected with a message
// naming both accepted shapes.
func ValidateNetworkSource(s string) error {
	if nfsSourceRe.MatchString(s) || cifsSourceRe.MatchString(s) {
		return nil
	}
	return fmt.Errorf("invalid network source %q: want host:/export (nfs-style) or //host/share (cifs-style)", s)
}

// MountChoice is one fully specified mount: one iteration of the mounts
// wizard loop, or the pre-fill derived from parsed CLI flags.
type MountChoice struct {
	Name        string
	Source      string
	Destination string
	Type        string
	// Options overrides the registry preset when non-nil; nil means
	// "registry preset applies" (identical to the CLI with no --option).
	Options []string
	// StartAt adds a .timer/.service pair when non-empty.
	StartAt string
	// State is "enabled" or "disabled"; empty defaults to enabled.
	State string
}

// spec converts the choice to a mount spec, defaulting the state.
func (c MountChoice) spec() profile.MountSpec {
	return profile.MountSpec{
		Source:      c.Source,
		Destination: c.Destination,
		Type:        c.Type,
		Options:     c.Options,
		StartAt:     c.StartAt,
		State:       defaultState(c.State),
	}
}

// defaultState applies the shared state default.
func defaultState(s string) string {
	if s == "" {
		return "enabled"
	}
	return s
}

// validate checks the choice early (the wizard wants to fail before the
// review screen, not inside WriteModule).
func (c MountChoice) validate(reg *generate.Registry) error {
	if c.Name == "" {
		return errors.New("mount: name is required")
	}
	if c.Source == "" {
		return fmt.Errorf("mount %q: source is required", c.Name)
	}
	if c.Destination == "" {
		return fmt.Errorf("mount %q: destination is required", c.Name)
	}
	if _, ok := reg.Entry(c.Type); !ok {
		return fmt.Errorf("mount %q: unknown filesystem type %q (not in the generate registry)", c.Name, c.Type)
	}
	switch c.State {
	case "", "enabled", "disabled":
	default:
		return fmt.Errorf("mount %q: state must be \"enabled\" or \"disabled\", got %q", c.Name, c.State)
	}
	return nil
}

// VolumeChoice is a detected local volume plus the wizard defaults
// derived from it: the kernel-recommended filesystem type, a default
// mount name, and a default destination.
type VolumeChoice struct {
	Volume generate.Volume
	// Type is the kernel-recommended registry entry for the volume's
	// filesystem family; empty when no entry is eligible.
	Type string
	// Name defaults to the volume label, else a short UUID prefix.
	Name string
	// Destination defaults to /mnt/<Name>.
	Destination string
}

// VolumeChoices converts detected volumes into wizard choices. The
// recommended type preselects the kernel-Recommended entry for the
// volume's fstype family (lsblk "ntfs" matches the ntfs family of
// ntfs3/ntfs-3g). Already-managed volumes keep their Managed mark for
// the label.
func VolumeChoices(reg *generate.Registry, vols []generate.Volume) []VolumeChoice {
	out := make([]VolumeChoice, 0, len(vols))
	for _, v := range vols {
		c := VolumeChoice{Volume: v}
		if rec, err := reg.Recommend(v.FSType); err == nil {
			c.Type = rec.Type
		}
		c.Name = v.Label
		if c.Name == "" {
			c.Name = "vol-" + shortUUID(v.UUID)
		}
		c.Destination = "/mnt/" + c.Name
		out = append(out, c)
	}
	return out
}

// shortUUID returns the first dash-separated UUID segment.
func shortUUID(uuid string) string {
	if i := strings.IndexByte(uuid, '-'); i > 0 {
		return uuid[:i]
	}
	if len(uuid) > 8 {
		return uuid[:8]
	}
	return uuid
}

// Label renders the choice for the volume picker, marking
// already-managed volumes.
func (c VolumeChoice) Label() string {
	v := c.Volume
	label := v.Label
	if label == "" {
		label = shortUUID(v.UUID)
	}
	s := fmt.Sprintf("%s (%s, %s)", label, v.FSType, v.Size)
	if v.Managed {
		s += " [managed]"
	}
	return s
}

// MountsWizard is the mounts spec-builder state machine: choices
// accumulate via AddMount and a single Write materializes the module.
type MountsWizard struct {
	sel    generate.Selection
	reg    *generate.Registry
	mounts map[string]profile.MountSpec
}

// NewMountsWizard starts a mounts wizard run for the target selection.
func NewMountsWizard(sel generate.Selection, reg *generate.Registry) *MountsWizard {
	return &MountsWizard{
		sel:    sel,
		reg:    reg,
		mounts: map[string]profile.MountSpec{},
	}
}

// Selection returns the wizard's target selection.
func (w *MountsWizard) Selection() generate.Selection { return w.sel }

// Mounts returns the accumulated mount specs (name order not
// guaranteed; use Input for the deterministic assembly).
func (w *MountsWizard) Mounts() map[string]profile.MountSpec { return w.mounts }

// AddMount validates and accumulates one mount choice; a later choice
// with the same name replaces the earlier one (matching the
// whole-entry-by-name merge of [mounts.<name>]).
func (w *MountsWizard) AddMount(c MountChoice) error {
	if err := c.validate(w.reg); err != nil {
		return err
	}
	w.mounts[c.Name] = c.spec()
	return nil
}

// Input assembles the final generate.Input through the shared assembly.
func (w *MountsWizard) Input(uid, gid int) generate.Input {
	return MountsInput(w.mounts, uid, gid)
}

// Write materializes the module with ONE WriteModule call carrying all
// accumulated mounts. uid/gid expand the registry's bare uid/gid option
// tokens (the interactive runner resolves them via os/user).
func (w *MountsWizard) Write(root string, uid, gid int) error {
	if len(w.mounts) == 0 {
		return errors.New("generate mounts: at least one mount is required")
	}
	if err := generate.WriteModule(root, w.sel, w.Input(uid, gid)); err != nil {
		return fmt.Errorf("generate mounts: %w", err)
	}
	return nil
}

// ShareChoice is one fully specified samba share: one iteration of the
// smb wizard shares loop.
type ShareChoice struct {
	Name    string
	Path    string
	Comment string
	// Writable and Public are per-share answers; the wizard defaults
	// writable=yes / public=no like the CLI flags.
	Writable bool
	Public   bool
}

// validate checks the share choice early.
func (c ShareChoice) validate() error {
	if c.Name == "" {
		return errors.New("share: name is required")
	}
	if c.Path == "" {
		return fmt.Errorf("share %q: path is required", c.Name)
	}
	return nil
}

// SmbServerChoice carries the server-level smb answers.
type SmbServerChoice struct {
	Group string
	Users []string
	// Avahi is nil for the default-on answer ("yes"); an explicit false
	// records avahi = false like --no-avahi.
	Avahi *bool
}

// SmbWizard is the smb spec-builder state machine: the server choice is
// set once, shares accumulate via AddShare, and a single Write
// materializes the module.
type SmbWizard struct {
	sel    generate.Selection
	server SmbServerChoice
	shares map[string]profile.ShareSpec
}

// NewSmbWizard starts an smb wizard run for the target selection.
func NewSmbWizard(sel generate.Selection) *SmbWizard {
	return &SmbWizard{sel: sel, shares: map[string]profile.ShareSpec{}}
}

// SetServer records the server-level answers.
func (w *SmbWizard) SetServer(c SmbServerChoice) { w.server = c }

// AddShare validates and accumulates one share choice.
func (w *SmbWizard) AddShare(c ShareChoice) error {
	if err := c.validate(); err != nil {
		return err
	}
	w.shares[c.Name] = profile.ShareSpec{
		Path:     c.Path,
		Comment:  c.Comment,
		Writable: c.Writable,
		Public:   c.Public,
	}
	return nil
}

// Input assembles the final generate.Input through the shared assembly.
func (w *SmbWizard) Input(defaultUser string, uid, gid int) generate.Input {
	return SmbInput(w.server.Group, w.server.Users, w.server.Avahi, w.shares, defaultUser, uid, gid)
}

// Write materializes the module with ONE WriteModule call.
func (w *SmbWizard) Write(root, defaultUser string, uid, gid int) error {
	if len(w.shares) == 0 {
		return errors.New("generate smb: at least one share is required")
	}
	if err := generate.WriteModule(root, w.sel, w.Input(defaultUser, uid, gid)); err != nil {
		return fmt.Errorf("generate smb: %w", err)
	}
	return nil
}

