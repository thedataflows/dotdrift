package cmd

import (
	"context"
	"io"
	"os"

	"github.com/thedataflows/dotdrift/internal/executil"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// StatusCmd reports drift between the resolved profile and the live system,
// plus the apply resume cursor. Read-only; exits 0 even when drift is found.
type StatusCmd struct {
	Profile string   `help:"Path to profile directory" type:"existingdir" default:"."`
	State   string   `help:"Path to state file" type:"path" default:""`
	Verbose bool     `help:"Show each probe as it starts ('checking <section>: <item>') on stderr" short:"v" default:"false"`
	Jobs    int      `help:"Concurrent probe workers (0 = number of CPUs)" short:"j" default:"0"`
	Diff    string   `help:"Show diff for files whose content differs; bare = internal diff, --diff=tool uses the named tool" default:""`
	Modules []string `arg:"" optional:"" name:"modules" help:"Limit scope to these modules (space or comma separated)"`
	out     io.Writer
	err     io.Writer
}

// Run loads state, resolves the plan, probes the live system, and prints
// the report through the service reads area (T-tui-reads): the service
// owns the read, the probe assembly (sudo elevation retry on permission
// denied, 0108), and the canonical renderers; the adapter wires its pinned
// seams and asks for the CLI's always-elevated behavior. Only real errors
// (detect/load/resolve/corrupt state) return non-nil; drift is reported,
// never an error.
func (c *StatusCmd) Run() error {
	area := service.NewReadsArea(service.ReadsDeps{
		Detect:        detectFacts,
		LoadProfile:   profileLoad,
		Resolve:       resolvePlan,
		OtherAccounts: otherAccounts,
		WarnLoad:      warnLoadNudges,
		PackagesFor:   packagesFor,
		NewMise:       defaultMise,
	})
	var verbose io.Writer
	if c.Verbose {
		verbose = c.err
		if verbose == nil {
			verbose = os.Stderr
		}
	}
	r, err := area.Status(context.Background(), service.StatusOpts{
		ProfilePath: c.Profile,
		StatePath:   c.State,
		Modules:     c.Modules,
		Jobs:        c.Jobs,
		Verbose:     verbose,
		Elevated:    true, // retry elevated on permission denied (system files)
	})
	if err != nil {
		return err
	}

	out := c.out
	if out == nil {
		out = os.Stdout
	}
	if err := service.RenderStatusHeader(out, r); err != nil {
		return err
	}
	if c.Diff != "" {
		entries, err := area.Diff(r.Plan, r.ProfileRoot)
		if err != nil {
			return err
		}
		if err := service.RenderDiff(out, entries, c.Diff, executil.ColorEnabled(out)); err != nil {
			return err
		}
	}
	service.RenderStatusNote(out, r)
	return nil
}

// otherAccounts lists the other OS accounts with configuration in the
// profile; a test seam (tests cannot reach profile's unexported lookup seam).
var otherAccounts = profile.OtherAccounts

