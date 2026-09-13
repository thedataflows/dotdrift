package cmd

import (
	"context"

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
	Profile    string `help:"Path to profile directory" type:"existingdir" default:"."`
	Compositor bool   `name:"compositor" hidden:"" help:"Run the experimental M15 compositor shell."`
}

// Run builds the reads and writes areas over the adapter's pinned seams
// and hands them to the shell through the TUI package's narrow
// interfaces (ADR-0008). WarnLoad stays nil: the tree's skip listing is
// the canonical surfacing for load-time nudges (the modules read's
// contract), so the TUI never doubles them as stderr lines. The config
// and writes areas are built lazily — they need the facts, which land
// with the first read.
func (c *TUICmd) Run() error {
	if c.Compositor {
		// The M15 compositor shell (issue 0073), mounted behind a hidden
		// flag until it reaches parity and the old shell is deleted
		// (T-tui-cleanup).
		return tui.NewCompositor(c.Profile).Run()
	}
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
		WritesFor: func(*facts.Facts) tui.Writes {
			return service.NewWritesArea(service.WritesDeps{
				Detect: detectFacts,
				NewMise: func(verbose bool) mise.Runner {
					m := defaultMise()
					m.Verbose = verbose
					return mise.NewExecMise(m)
				},
			})
		},
		ApplyFor: func(*facts.Facts) tui.ApplyLauncher {
			return tuiApplyLauncher{area: service.NewApplyArea(service.ApplyDeps{
				Detect:      detectFacts,
				LoadProfile: profileLoad,
				Resolve:     resolvePlan,
				NewMise:     func() *mise.Mise { return defaultMise() },
			})}
		},
	})
	return runTUIProgram(shell)
}

// tuiApplyLauncher adapts the apply area to the shell's launcher
// interface (Go has no return-type covariance: Start's concrete
// *ApplySession satisfies ApplyRun, but the method signature doesn't).
type tuiApplyLauncher struct{ area *service.ApplyArea }

func (l tuiApplyLauncher) Preview(opts service.ApplyOpts) ([]service.StepPreview, error) {
	return l.area.Preview(opts)
}

func (l tuiApplyLauncher) Start(ctx context.Context, opts service.ApplyOpts) (tui.ApplyRun, error) {
	sess, err := l.area.Start(ctx, opts)
	if err != nil {
		return nil, err
	}
	return sess, nil
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
