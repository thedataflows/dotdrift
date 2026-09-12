package cmd

import (
	"io"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/tui"
)

// TUICmd opens the two-pane interactive shell (M14, issue 0063's
// approved prototype): the one interactive home (ADR-0007). It is the
// only command that runs a TUI; everything else is strict flag mode.
type TUICmd struct {
	Profile string `help:"Path to profile directory" type:"existingdir" default:"."`
	// out/err exist for symmetry with the other adapters; the TUI owns
	// its terminal.
	out io.Writer `kong:"-"`
	err io.Writer `kong:"-"`
}

// Run builds the reads area over the adapter's pinned seams and hands it
// to the shell through the TUI package's narrow interface (ADR-0008).
// WarnLoad stays nil: the tree's skip listing is the canonical surfacing
// for load-time nudges (the modules read's contract), so the TUI never
// doubles them as stderr lines. The config area is built lazily — the
// editor suite needs the facts, which land with the first read.
func (c *TUICmd) Run() error {
	area := service.NewReadsArea(service.ReadsDeps{
		Detect:        detectFacts,
		LoadProfile:   profileLoad,
		Resolve:       resolvePlan,
		OtherAccounts: otherAccounts,
	})
	shell := tui.New(area, tui.Options{
		ProfilePath: c.Profile,
		ProbesFor:   tuiProbesFor,
		ConfigFor: func(f *facts.Facts) service.ConfigEditor {
			return service.NewConfigArea(c.Profile, service.ConfigDeps{Facts: f})
		},
	})
	return runTUIProgram(shell)
}

// tuiProbesFor builds the drift probes for the status view: the same
// backend/mise seams and sudo-elevation retry the `status` adapter uses.
func tuiProbesFor(f *facts.Facts) drift.Probes {
	pr := drift.DefaultProbes()
	pr.IsInstalled = packagesFor(f.Backend).IsInstalled
	pr.ToolCurrent = mise.NewExecMise(defaultMise()).Current
	return elevateProbes(pr)
}

// runTUIProgram is the program runner seam; tests swap it so the command
// can be exercised without a terminal.
var runTUIProgram = func(m *tui.Shell) error {
	return m.Run()
}
