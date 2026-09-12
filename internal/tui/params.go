package tui

import (
	"github.com/thedataflows/dotdrift/internal/generate"
)

// Wizard run parameters: plain values carried over the cmd seam (no cmd
// import — that would be a cycle). The parsed input flags pre-fill the
// wizard's defaults; everything empty means "fresh run".

// MountsParams starts a mounts wizard run.
type MountsParams struct {
	// Target selection inputs (the generate flags, not wizard input
	// flags): empty layer means "ask", empty module id defaults in the
	// form.
	Profile  string
	Layer    string
	ModuleID string
	Hostname string
	Username string

	// Input-flag pre-fill for the first mount iteration.
	Name        string
	Source      string
	Destination string
	Type        string
	Options     []string
	StartAt     string
	State       string
}

// Prefill derives the first mount iteration's defaults from the parsed
// input flags.
func (p MountsParams) Prefill() generate.MountChoice {
	return generate.MountChoice{
		Name:        p.Name,
		Source:      p.Source,
		Destination: p.Destination,
		Type:        p.Type,
		Options:     p.Options,
		StartAt:     p.StartAt,
		State:       p.State,
	}
}

// SmbParams starts an smb wizard run.
type SmbParams struct {
	Profile  string
	Layer    string
	ModuleID string
	Hostname string
	Username string

	// Input-flag pre-fill.
	Group    string
	Users    []string
	Avahi    *bool
	Shares   []string // raw "name=path" values, like the --share flag
	Writable *bool
	Readonly bool
	Public   bool
}

// PrefillShares decodes the raw share flags into wizard share choices
// with the flag-resolved writable/public applied, mirroring the CLI's
// assembly. An empty Shares list yields nil (the shares loop starts
// fresh).
func (p SmbParams) PrefillShares() ([]generate.ShareChoice, error) {
	if len(p.Shares) == 0 {
		return nil, nil
	}
	specs, err := generate.ParseShareFlags(p.Shares, generate.ResolveWritable(p.Writable, p.Readonly), p.Public)
	if err != nil {
		return nil, err
	}
	out := make([]generate.ShareChoice, 0, len(specs))
	for name, spec := range specs {
		out = append(out, generate.ShareChoice{
			Name:     name,
			Path:     spec.Path,
			Writable: spec.Writable,
			Public:   spec.Public,
		})
	}
	return out, nil
}

// smbParamsFromMounts carries the target selection across a mounts → smb
// tab switch; the smb wizard's own default module id applies.
func smbParamsFromMounts(p MountsParams) SmbParams {
	return SmbParams{Profile: p.Profile, Layer: p.Layer, Hostname: p.Hostname, Username: p.Username}
}

// mountsParamsFromSmb carries the target selection across an smb →
// mounts tab switch; the mounts wizard's own default module id applies.
func mountsParamsFromSmb(p SmbParams) MountsParams {
	return MountsParams{Profile: p.Profile, Layer: p.Layer, Hostname: p.Hostname, Username: p.Username}
}
