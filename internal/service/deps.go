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

func defaultApplyDeps() ApplyDeps {
	return ApplyDeps{
		Detect:          detect.Detect,
		LoadProfile:     profile.Load,
		Resolve:         resolve.Resolve,
		NewMise:         mise.DefaultMise,
		PackagesFor:     packages.For,
		NewSmbRunner:    func() smb.Runner { return &smb.ExecRunner{} },
		StdinIsTerminal: executil.IsStdinTerminal,
	}
}
