package cmd

import (
	"context"

	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/tui"
)

// profileLoadTolerant loads the profile for interactive use: a broken
// module.toml greys out its row instead of killing the shell (M15). The
// write paths (plan/apply) keep the strict profileLoad.
var profileLoadTolerant = profile.LoadTolerant

// TUICmd opens the interactive shell (M15, issue 0073): the one
// interactive home (ADR-0007). It is the only command that runs a TUI;
// everything else is strict flag mode.
type TUICmd struct {
	Profile string `help:"Path to profile directory" type:"existingdir" default:"."`
}

// Run builds the reads area over the adapter's pinned seams and hands it
// to the compositor through the TUI package's narrow interfaces
// (ADR-0008). The reads area loads tolerantly — a broken module.toml
// greys out its row instead of killing the shell (strict Load stays the
// plan/apply path); the config and writes areas are built lazily — they
// need the facts, which land with the first read. The sudo checker is
// executil.SudoValidate (the elevation modal's seam).
func (c *TUICmd) Run() error {
	area := service.NewReadsArea(service.ReadsDeps{
		Detect:        detectFacts,
		LoadProfile:   profileLoadTolerant,
		Resolve:       resolvePlan,
		OtherAccounts: otherAccounts,
	})
	comp := tui.NewCompositor(area, c.Profile, func(f *facts.Facts) tui.LayerReader {
		return service.NewConfigArea(c.Profile, service.ConfigDeps{Facts: f})
	})
	comp.SetApply(func(*facts.Facts) tui.ApplyLauncher {
		return tuiApplyLauncher{area: service.NewApplyArea(service.ApplyDeps{
			Detect:      detectFacts,
			LoadProfile: profileLoad,
			Resolve:     resolvePlan,
			NewMise:     func() *mise.Mise { return defaultMise() },
		})}
	}, executil.SudoValidate)
	comp.SetWrites(func(*facts.Facts) tui.Writes {
		return service.NewWritesArea(service.WritesDeps{
			Detect: detectFacts,
			NewMise: func(verbose bool) mise.Runner {
				m := defaultMise()
				m.Verbose = verbose
				return mise.NewExecMise(m)
			},
		})
	})
	return runTUIProgram(comp)
}

// tuiApplyLauncher adapts the apply area to the compositor's launcher
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

// runTUIProgram is the program runner seam; tests swap it so the command
// can be exercised without a terminal.
var runTUIProgram = func(m *tui.Compositor) error {
	return m.Run()
}
