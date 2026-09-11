package service

import (
	"github.com/thedataflows/dotdrift/internal/detect"
	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
)

// ApplyDeps carries the apply session's construction seams: every
// OS-touching entry point the orchestration needs, swappable by tests and
// by the CLI adapter (the same seams cmd/apply.go keeps as package vars
// until 0070 deletes that copy). Zero-value fields are replaced by the
// real implementations by NewApplyArea.
type ApplyDeps struct {
	Detect       func() (*facts.Facts, error)
	LoadProfile  func(root string, f *facts.Facts) (*profile.Profile, error)
	Resolve      func(p *profile.Profile, f *facts.Facts) (*resolve.Plan, error)
	NewMise      func() *mise.Mise
	PackagesFor  func(backend string) packages.Backend
	NewSmbRunner func() smb.Runner
	// StdinIsTerminal reports whether the consumer's stdin is a terminal.
	// It keys the interactive-hook opt-in at config-write (0064-D4): the
	// CLI passes its own reality; a UI that can hand the terminal over
	// sets ApplyOpts.HandoverAvailable instead.
	StdinIsTerminal func() bool
}

// WithDefaults returns the dep set with zero-value fields replaced by the
// real implementations. Consumers composing partial dep sets (the CLI
// adapter overrides only what it wraps) get the real behavior for the rest.
func (d ApplyDeps) WithDefaults() ApplyDeps {
	if d.Detect == nil {
		d.Detect = detect.Detect
	}
	if d.LoadProfile == nil {
		d.LoadProfile = profile.Load
	}
	if d.Resolve == nil {
		d.Resolve = resolve.Resolve
	}
	if d.NewMise == nil {
		d.NewMise = mise.DefaultMise
	}
	if d.PackagesFor == nil {
		d.PackagesFor = packages.For
	}
	if d.NewSmbRunner == nil {
		d.NewSmbRunner = func() smb.Runner { return &smb.ExecRunner{} }
	}
	if d.StdinIsTerminal == nil {
		d.StdinIsTerminal = executil.IsStdinTerminal
	}
	return d
}
