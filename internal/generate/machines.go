package generate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// Spec-builder state machines for the generate command: MountsWizard /
// SmbWizard accumulate validated choices and assemble Input through the
// same builders the CLI flag path uses (MountsInput / SmbInput /
// ParseShareFlags). Both consumers produce byte-identical module trees
// for the same logical inputs (contract invariant 15).
//
// Write semantics: the machines accumulate everything and call
// WriteModule ONCE (WriteModule replaces [mounts]/[smb] wholesale, so a
// mid-loop write would wipe earlier iterations). An aborted flow writes
// nothing.

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

// MountChoice is one fully specified mount: one iteration of the
// interactive mounts loop, or the pre-fill derived from parsed CLI
// flags.
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
		State:       DefaultState(c.State),
	}
}

// DefaultState applies the shared state default: an empty state means
// "enabled".
func DefaultState(s string) string {
	if s == "" {
		return "enabled"
	}
	return s
}

// validate checks the choice early (an interactive producer wants to
// fail before the review screen, not inside WriteModule).
func (c MountChoice) validate(reg *Registry) error {
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

// VolumeChoice is a detected local volume plus the defaults derived
// from it: the kernel-recommended filesystem type, a default mount
// name, and a default destination.
type VolumeChoice struct {
	Volume Volume
	// Type is the kernel-recommended registry entry for the volume's
	// filesystem family; empty when no entry is eligible.
	Type string
	// Name defaults to the volume label, else a short UUID prefix.
	Name string
	// Destination defaults to /mnt/<Name>.
	Destination string
}

// VolumeChoices converts detected volumes into choices for an
// interactive producer. The recommended type preselects the
// kernel-Recommended entry for the volume's fstype family (lsblk "ntfs"
// matches the ntfs family of ntfs3/ntfs-3g). Already-managed volumes
// keep their Managed mark for the label.
func VolumeChoices(reg *Registry, vols []Volume) []VolumeChoice {
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

// Label renders the choice for a volume picker, marking already-managed
// volumes.
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
	sel    Selection
	reg    *Registry
	mounts map[string]profile.MountSpec
}

// NewMountsWizard starts a mounts wizard run for the target selection.
func NewMountsWizard(sel Selection, reg *Registry) *MountsWizard {
	return &MountsWizard{
		sel:    sel,
		reg:    reg,
		mounts: map[string]profile.MountSpec{},
	}
}

// Selection returns the wizard's target selection.
func (w *MountsWizard) Selection() Selection { return w.sel }

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

// Input assembles the final Input through the shared assembly.
func (w *MountsWizard) Input(uid, gid int) Input {
	return MountsInput(w.mounts, uid, gid)
}

// Write materializes the module with ONE WriteModule call carrying all
// accumulated mounts. uid/gid expand the registry's bare uid/gid option
// tokens (the interactive runner resolves them via os/user).
func (w *MountsWizard) Write(root string, uid, gid int) error {
	if len(w.mounts) == 0 {
		return errors.New("generate mounts: at least one mount is required")
	}
	if err := WriteModule(root, w.sel, w.Input(uid, gid)); err != nil {
		return fmt.Errorf("generate mounts: %w", err)
	}
	return nil
}

// ShareChoice is one fully specified samba share: one iteration of the
// smb shares loop.
type ShareChoice struct {
	Name    string
	Path    string
	Comment string
	// Writable and Public are per-share answers; the defaults are
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
	sel    Selection
	server SmbServerChoice
	shares map[string]profile.ShareSpec
}

// NewSmbWizard starts an smb wizard run for the target selection.
func NewSmbWizard(sel Selection) *SmbWizard {
	return &SmbWizard{sel: sel, shares: map[string]profile.ShareSpec{}}
}

// SetServer records the server-level answers.
func (w *SmbWizard) SetServer(c SmbServerChoice) { w.server = c }

// Shares returns the accumulated share specs (name order not
// guaranteed; use Input for the deterministic assembly).
func (w *SmbWizard) Shares() map[string]profile.ShareSpec { return w.shares }

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

// Input assembles the final Input through the shared assembly.
func (w *SmbWizard) Input(defaultUser string, uid, gid int) Input {
	return SmbInput(w.server.Group, w.server.Users, w.server.Avahi, w.shares, defaultUser, uid, gid)
}

// Write materializes the module with ONE WriteModule call.
func (w *SmbWizard) Write(root, defaultUser string, uid, gid int) error {
	if len(w.shares) == 0 {
		return errors.New("generate smb: at least one share is required")
	}
	if err := WriteModule(root, w.sel, w.Input(defaultUser, uid, gid)); err != nil {
		return fmt.Errorf("generate smb: %w", err)
	}
	return nil
}
